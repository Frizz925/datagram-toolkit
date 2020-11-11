package udp

import (
	"bufio"
	"bytes"
	"datagram-toolkit/udp/protocol"
	"datagram-toolkit/util"
	uatomic "datagram-toolkit/util/atomic"
	"io"
	"net"
	"sync"
	"sync/atomic"
)

type streamRecvPending struct {
	off        int
	data, head []byte
	isFin      bool
}

type streamSendRequest struct {
	flags, cmd uint8
	data, head []byte
	result     chan<- streamSendResult
}

type streamSendResult struct {
	n   int
	err error
}

type Stream struct {
	net.Conn

	reader *bufio.Reader
	writer *bufio.Writer

	windowSize     uint32
	frameSize      uint32
	peerWindowSize uint32

	window []byte

	recvCh     chan []byte
	recvPool   *util.BufferPool
	recvErrCh  chan error
	recvErr    atomic.Value
	recvOffset uint32
	recvBuffer atomic.Value

	sendCh   chan streamSendRequest
	sendPool *util.BufferPool

	readBuffer *bytes.Buffer
	readLock   sync.Mutex

	writeLock sync.Mutex

	handshakeNotify chan struct{}
	handshakeDone   uatomic.Bool
	handshakeLock   sync.Mutex

	resetNotify chan struct{}

	wg sync.WaitGroup

	die       chan struct{}
	closeOnce sync.Once
}

func NewStream(conn net.Conn, cfg StreamConfig) *Stream {
	cfg = sanitizeStreamConfig(cfg)
	s := &Stream{
		Conn: conn,

		reader: bufio.NewReaderSize(conn, cfg.ReadBufferSize),
		writer: bufio.NewWriterSize(conn, cfg.WriteBufferSize),

		windowSize:     uint32(cfg.WindowSize),
		frameSize:      uint32(cfg.PeerConfig.FrameSize),
		peerWindowSize: uint32(cfg.PeerConfig.WindowSize),

		window: make([]byte, cfg.WindowSize),

		recvCh:    make(chan []byte, cfg.ReadBacklog),
		recvPool:  util.NewBufferPool(cfg.ReadBufferSize, cfg.ReadBacklog),
		recvErrCh: make(chan error, 1),

		sendCh:   make(chan streamSendRequest, cfg.WriteBacklog),
		sendPool: util.NewBufferPool(cfg.WriteBufferSize, cfg.WriteBacklog),

		readBuffer: bytes.NewBuffer(make([]byte, 0, cfg.ReadBufferSize)),

		handshakeNotify: make(chan struct{}, 1),
		resetNotify:     make(chan struct{}),

		die: make(chan struct{}),
	}
	if s.frameSize > 0 && s.peerWindowSize > 0 {
		s.handshakeDone.Set(true)
	}
	s.recvBuffer.Store([]streamRecvPending{})
	s.wg.Add(2)
	go s.recvRoutine()
	go s.sendRoutine()
	return s
}

func (s *Stream) Handshake() error {
	if s.handshakeDone.Get() {
		return nil
	}
	if err := s.getReadError(); err != nil {
		return err
	}
	s.handshakeLock.Lock()
	defer s.handshakeLock.Unlock()
	if err := s.sendHandshake(); err != nil {
		return err
	}
	select {
	case <-s.handshakeNotify:
		s.handshakeDone.Set(true)
		return nil
	case err := <-s.recvErrCh:
		return err
	case <-s.resetNotify:
		return io.EOF
	case <-s.die:
		return io.EOF
	}
}

func (s *Stream) Read(b []byte) (int, error) {
	if err := s.getReadError(); err != nil {
		return 0, err
	}
	s.readLock.Lock()
	defer s.readLock.Unlock()
	for {
		if s.readBuffer.Len() > 0 {
			return s.readBuffer.Read(b)
		}
		select {
		case p := <-s.recvCh:
			s.readBuffer.Write(p)
		case err := <-s.recvErrCh:
			return 0, err
		case <-s.resetNotify:
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
	fsize := int(atomic.LoadUint32(&s.frameSize))
	wsize := int(atomic.LoadUint32(&s.peerWindowSize))
	if len(b) > wsize {
		b = b[:wsize]
	}
	w := 0
	for len(b) > 0 {
		bs := b
		flags := uint8(0)
		if len(bs) > fsize {
			bs = b[:fsize]
		} else {
			flags = protocol.FlagFIN
		}
		n, err := s.sendData(flags, w, bs)
		if err != nil {
			return w, err
		}
		b = b[n:]
		w += n
	}
	return w, nil
}

func (s *Stream) Reset() error {
	return s.internalReset(true)
}

func (s *Stream) Close() error {
	return s.internalClose(true)
}

func (s *Stream) internalReset(sendRst bool) error {
	if sendRst {
		if err := s.sendDataRst(); err != nil {
			return err
		}
	}
	s.recvBuffer.Store([]streamRecvPending{})
	atomic.StoreUint32(&s.recvOffset, 0)
	util.AsyncNotify(s.resetNotify)
	return nil
}

func (s *Stream) internalClose(sendFin bool) error {
	if sendFin {
		if err := s.sendClose(); err != nil {
			return err
		}
	}
	s.closeOnce.Do(func() {
		close(s.die)
	})
	if err := s.Conn.Close(); err != nil {
		return err
	}
	s.wg.Wait()
	return nil
}
