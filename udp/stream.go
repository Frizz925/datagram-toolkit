package udp

import (
	"bufio"
	"bytes"
	"datagram-toolkit/util"
	uatomic "datagram-toolkit/util/atomic"
	"encoding/binary"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
)

type streamWriteRequest struct {
	flags  uint8
	cmd    uint8
	data   []byte
	head   []byte
	result chan<- streamWriteResult
}

type streamWriteResult struct {
	n   int
	err error
}

type streamPendingAck struct {
	flags uint8
	off   int
	data  []byte
}

type Stream struct {
	net.Conn

	logger *log.Logger

	reader     *bufio.Reader
	readerLock sync.Mutex

	writer     *bufio.Writer
	writerLock sync.Mutex

	// For generating frame sequence number
	seq uint32
	// Our window for receving stream of data
	window []byte
	// The size of our window
	windowSize uint32
	// The size of a single frame which should be the same for both peers
	frameSize uint32
	// The size of peer's window
	peerWindowSize uint32

	rttStats RTTStats

	streamSeqs     map[uint16]bool
	streamRead     uint32
	streamSize     uint32
	streamRecvLock sync.Mutex

	streamRttSeq   uint32
	streamRttSize  uint32
	streamAcks     []uint16
	streamAckMap   map[uint16]streamPendingAck
	streamSendLock sync.Mutex

	retransmitNotify chan struct{}

	buffer     *bytes.Buffer
	buffers    *util.BufferPool
	bufferLock sync.Mutex

	handshakeCh   chan error
	handshakeDone uatomic.Bool
	handshakeLock sync.Mutex

	readCh    chan []byte
	readErr   atomic.Value
	readErrCh chan error
	readRstCh chan struct{}
	readLock  sync.Mutex

	writeCh    chan streamWriteRequest
	writeRstCh chan struct{}
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
		Conn:   conn,
		logger: cfg.Logger,
		reader: bufio.NewReaderSize(conn, cfg.ReadBufferSize),
		writer: bufio.NewWriterSize(conn, cfg.WriteBufferSize),

		window:         make([]byte, cfg.WindowSize),
		windowSize:     uint32(cfg.WindowSize),
		frameSize:      uint32(cfg.PeerConfig.FrameSize),
		peerWindowSize: uint32(cfg.PeerConfig.WindowSize),

		streamSeqs:   make(map[uint16]bool),
		streamAcks:   make([]uint16, 0, cfg.ReadBacklog),
		streamAckMap: make(map[uint16]streamPendingAck),

		retransmitNotify: make(chan struct{}),

		buffer:  bytes.NewBuffer(make([]byte, 0, cfg.WindowSize)),
		buffers: util.NewBufferPool(cfg.ReadBufferSize, cfg.ReadBacklog),

		handshakeCh: make(chan error, 1),

		readCh:    make(chan []byte, cfg.ReadBacklog),
		readErrCh: make(chan error, 1),
		readRstCh: make(chan struct{}),

		writeCh:    make(chan streamWriteRequest, cfg.WriteBacklog),
		writeRstCh: make(chan struct{}),

		resetCh: make(chan error, 1),

		die: make(chan struct{}),
	}
	// Assume we've done handshake if peer configurations are set
	if s.frameSize > 0 && s.peerWindowSize > 0 {
		s.handshakeDone.Set(true)
	}
	s.wg.Add(3)
	go s.readRoutine()
	go s.writeRoutine()
	go s.retransmitRoutine()
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
	binary.BigEndian.PutUint32(buf, s.windowSize)
	s.rttStats.UpdateSend()
	if _, err := s.write(0, cmdSYN, buf[:]); err != nil {
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
	for {
		s.bufferLock.Lock()
		if s.buffer.Len() > 0 {
			n, err := s.buffer.Read(b)
			s.bufferLock.Unlock()
			return n, err
		}
		s.bufferLock.Unlock()
		select {
		case data := <-s.readCh:
			n := copy(b, data)
			if n < len(data) {
				s.bufferLock.Lock()
				s.buffer.Write(data[n:])
				s.bufferLock.Unlock()
			}
			return n, nil
		case err := <-s.readErrCh:
			return 0, err
		case <-s.readRstCh:
			return 0, io.EOF
		case <-s.die:
			return 0, io.EOF
		}
	}
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

	finSent := false
	s.streamSendLock.Lock()
	defer func() {
		s.streamSendLock.Unlock()
		// FIN frame not sent, tell peer to reset stream
		if !finSent {
			s.writeAsync(0, cmdRST)
		}
	}()

	w := 0
	for len(b) > 0 {
		var (
			bs    []byte
			flags uint8
		)
		if fsize >= len(b) {
			bs = b
			flags = flagFIN
		} else {
			bs = b[:fsize]
		}
		n, err := s.sendData(flags, w, bs)
		if err != nil {
			return w, err
		}
		if !finSent && flags == flagFIN {
			finSent = true
		}
		b = b[n:]
		w += n
	}

	return w, nil
}

func (s *Stream) Reset() error {
	s.resetLock.Lock()
	defer s.resetLock.Unlock()
	if _, err := s.write(0, cmdRST); err != nil {
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

func (s *Stream) nextSeq() uint16 {
	return uint16(atomic.AddUint32(&s.seq, 1))
}

func (s *Stream) writeAsync(flags, cmd uint8, body ...[]byte) <-chan streamWriteResult {
	var req streamWriteRequest
	req.flags = flags
	req.cmd = cmd

	ch := make(chan streamWriteResult, 1)
	req.result = ch

	if len(body) > 0 {
		buf := s.buffers.Get()
		n := 0
		for _, b := range body {
			nn := copy(buf[n:], b)
			n += nn
		}
		req.data = buf[:n]
		req.head = buf
	}

	s.writeCh <- req
	return ch
}

func (s *Stream) write(flags, cmd uint8, body ...[]byte) (int, error) {
	select {
	case res := <-s.writeAsync(flags, cmd, body...):
		return res.n, res.err
	case <-s.writeRstCh:
		return 0, io.EOF
	case <-s.die:
		return 0, io.EOF
	}
}

func (s *Stream) sendData(flags uint8, off int, b []byte) (int, error) {
	seq := s.nextSeq()
	bn := len(b)
	sdh := newStreamDataHdr(seq, uint32(off), uint32(bn))
	fn, err := s.write(flags, cmdPSH, sdh[:], b)
	if err != nil {
		return 0, err
	}
	// Bytes written returned by write is the amount including the headers
	// We need to deduct the stream frame header
	n := fn - szStreamDataHdr

	rttSize := int(atomic.LoadUint32(&s.streamRttSize))
	if n > rttSize {
		atomic.StoreUint32(&s.streamRttSize, uint32(n))
		atomic.StoreUint32(&s.streamRttSeq, uint32(seq))
		s.rttStats.UpdateSend()
	}

	buf := make([]byte, n)
	copy(buf, b)
	s.streamAcks = append(s.streamAcks, seq)
	s.streamAckMap[seq] = streamPendingAck{
		flags: flags,
		off:   off,
		data:  buf,
	}

	return n, nil
}

func (s *Stream) getReadError() error {
	if err, ok := s.readErr.Load().(error); ok {
		return err
	}
	return nil
}
