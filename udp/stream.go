package udp

import (
	"bufio"
	"bytes"
	"datagram-toolkit/util"
	uatomic "datagram-toolkit/util/atomic"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"sync/atomic"
)

type streamReadResult struct {
	seq  uint16
	off  int
	data []byte
	head []byte
}

type streamWriteRequest struct {
	data   []byte
	flush  bool
	result chan<- streamWriteResult
}

type streamWriteResult struct {
	n   int
	err error
}

type Stream struct {
	net.Conn

	reader     *bufio.Reader
	readerLock sync.Mutex

	writer     *bufio.Writer
	writerLock sync.Mutex

	// For generating frame sequence number
	seq uint32
	// The size of our window
	windowSize int
	// The size of a single frame which should be the same for both peers
	frameSize uint32
	// The size of peer's window
	peerWindowSize uint32

	buffer     *bytes.Buffer
	buffers    *util.BufferPool
	bufferLock sync.Mutex

	handshakeCh   chan error
	handshakeDone uatomic.Bool
	handshakeLock sync.Mutex

	readCh    chan streamReadResult
	readAckCh chan streamDataAck
	readFinCh chan int
	readErr   atomic.Value
	readErrCh chan error
	readLock  sync.Mutex

	writeCh    chan streamWriteRequest
	writeErrCh chan error
	writeLock  sync.Mutex

	resetCh   chan error
	resetLock sync.Mutex

	wg  sync.WaitGroup
	mu  sync.Mutex
	die chan struct{}

	closeOnce sync.Once
}

func NewStream(conn net.Conn, cfg StreamConfig) *Stream {
	cfg = sanitizeStreamConfig(cfg)
	s := &Stream{
		Conn:           conn,
		reader:         bufio.NewReaderSize(conn, cfg.ReadBufferSize),
		writer:         bufio.NewWriterSize(conn, cfg.WriteBufferSize),
		windowSize:     cfg.WindowSize,
		frameSize:      uint32(cfg.PeerConfig.FrameSize),
		peerWindowSize: uint32(cfg.PeerConfig.WindowSize),
		buffer:         bytes.NewBuffer(make([]byte, 0, cfg.WindowSize)),
		buffers:        util.NewBufferPool(cfg.ReadBufferSize, cfg.ReadBacklog),
		handshakeCh:    make(chan error),
		readCh:         make(chan streamReadResult, cfg.ReadBacklog),
		readAckCh:      make(chan streamDataAck, cfg.ReadBacklog),
		readFinCh:      make(chan int, 1),
		readErrCh:      make(chan error),
		writeCh:        make(chan streamWriteRequest, cfg.WriteBacklog),
		writeErrCh:     make(chan error),
		resetCh:        make(chan error),
		die:            make(chan struct{}),
	}
	// Assume we've done handshake if peer configurations are set
	if s.frameSize > 0 && s.peerWindowSize > 0 {
		s.handshakeDone.Set(true)
	}
	s.wg.Add(2)
	go s.readRoutine()
	go s.writeRoutine()
	return s
}

// Initiate handshake with peer in order to gain both peer's own and shared configurations
func (s *Stream) Handshake() error {
	s.handshakeLock.Lock()
	defer s.handshakeLock.Unlock()
	// Handshake already done, no need to do anything
	if s.handshakeDone.Get() {
		return nil
	}
	if err := s.getReadError(); err != nil {
		return err
	}

	buf := s.buffers.Get()
	defer s.buffers.Put(buf)
	binary.BigEndian.PutUint32(buf, uint32(s.windowSize))
	if _, err := s.writeFrame(0, cmdSYN, buf[:]); err != nil {
		return err
	}

	select {
	case err := <-s.handshakeCh:
		if err != nil {
			return err
		}
		s.handshakeDone.Set(true)
		return nil
	case err := <-s.readErrCh:
		return err
	case <-s.die:
		return io.EOF
	}
}

func (s *Stream) Read(b []byte) (int, error) {
	s.readLock.Lock()
	defer s.readLock.Unlock()

	if s.bufferLen() >= len(b) {
		return s.bufferRead(b)
	}

	wsize := s.windowSize
	window := s.buffers.Get()
	defer s.buffers.Put(window)
	for r := 0; r < wsize; {
		select {
		case res := <-s.readCh:
			if res.off >= len(window) {
				return 0, io.ErrShortBuffer
			}
			n := copy(window[res.off:], res.data)
			r += n
			s.buffers.Put(res.head)
			sda := newStreamDataAck(res.seq, uint32(r))
			if _, err := s.writeFrame(flagACK, cmdPSH, sda[:]); err != nil {
				return 0, err
			}
		case n := <-s.readFinCh:
			wsize = n
		case err := <-s.readErrCh:
			return 0, err
		case <-s.resetCh:
			s.internalReset()
			return 0, io.EOF
		case <-s.die:
			return 0, io.EOF
		}
	}

	s.bufferLock.Lock()
	s.buffer.Write(window[:wsize])
	n, err := s.buffer.Read(b)
	s.bufferLock.Unlock()
	return n, err
}

func (s *Stream) Write(b []byte) (int, error) {
	s.writeLock.Lock()
	defer s.writeLock.Unlock()
	if err := s.Handshake(); err != nil {
		return 0, err
	}

	fsize := int(atomic.LoadUint32(&s.frameSize)) - szStreamDataHdr
	wsize := int(atomic.LoadUint32(&s.peerWindowSize))
	if len(b) > wsize {
		b = b[:wsize]
	}

	w := 0
	hdrs := make([]streamDataHdr, 0, len(b)/fsize)
	for seq := uint16(1); len(b) > 0; seq++ {
		var bs []byte
		if fsize > len(b) {
			bs = b
		} else {
			bs = b[:fsize]
		}
		bsn := len(bs)
		sdh := newStreamDataHdr(seq, uint32(w), uint32(bsn))
		n, err := s.writeFrame(0, cmdPSH, sdh[:], bs)
		if err != nil {
			return w, err
		}
		hdrs = append(hdrs, sdh)
		b = b[n:]
		w += n
	}

	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], uint32(w))
	if _, err := s.writeFrame(flagFIN, cmdPSH, buf[:]); err != nil {
		return w, err
	}

	var emptySdh streamDataHdr
	for acked := 0; acked < len(hdrs); {
		select {
		case sda := <-s.readAckCh:
			idx := int(sda.Seq() - 1)
			if idx >= len(hdrs) {
				continue
			}
			hdrs[idx] = emptySdh
			acked++
		case err := <-s.readErrCh:
			return w, err
		case <-s.resetCh:
			return w, io.EOF
		case <-s.die:
			return w, io.EOF
		}
	}

	return w, nil
}

func (s *Stream) Reset() error {
	s.resetLock.Lock()
	defer s.resetLock.Unlock()
	if _, err := s.writeFrame(0, cmdRST); err != nil {
		return err
	}
	var err error
	select {
	case err = <-s.resetCh:
	case err = <-s.readErrCh:
	case <-s.die:
		err = io.EOF
	}
	s.internalReset()
	return err
}

func (s *Stream) Close() error {
	return s.internalClose(true)
}

func (s *Stream) readRoutine() {
	for {
		err := s.internalRead()
		if err != nil {
			s.wg.Done()
			s.handleReadError(err)
			return
		}
	}
}

func (s *Stream) writeRoutine() {
	for {
		select {
		case req := <-s.writeCh:
			var res streamWriteResult
			res.n, res.err = s.internalWrite(req.data, req.flush)
			req.result <- res
		case <-s.die:
			s.wg.Done()
			return
		}
	}
}

func (s *Stream) write(b []byte, flush bool) (int, error) {
	for {
		select {
		case res := <-s.writeAsync(b, flush):
			return res.n, res.err
		case err := <-s.writeErrCh:
			return 0, err
		case <-s.die:
			return 0, io.EOF
		}
	}
}

func (s *Stream) writeAsync(b []byte, flush bool) <-chan streamWriteResult {
	ch := make(chan streamWriteResult, 1)
	s.writeCh <- streamWriteRequest{
		data:   b,
		flush:  flush,
		result: ch,
	}
	return ch
}

func (s *Stream) writeFrame(flags, cmd uint8, body ...[]byte) (int, error) {
	seq := uint16(atomic.LoadUint32(&s.seq))
	hdr := newStreamHdr(seq, flags, cmd)
	buf := s.buffers.Get()
	defer s.buffers.Put(buf)
	n := copy(buf, hdr[:])
	for _, b := range body {
		nn := copy(buf[n:], b)
		n += nn
	}
	w, err := s.write(buf[:n], true)
	if err != nil {
		return 0, err
	}
	return w - len(hdr), nil
}

func (s *Stream) getReadError() error {
	if err, ok := s.readErr.Load().(error); ok {
		return err
	}
	return nil
}
