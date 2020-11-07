package udp

import (
	"bytes"
	"datagram-toolkit/util"
	uatomic "datagram-toolkit/util/atomic"
	uerrors "datagram-toolkit/util/errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type sessionConfig struct {
	Config
	listener *Listener
	raddr    *net.UDPAddr
}

type Session struct {
	listener *Listener
	raddr    *net.UDPAddr

	readCh       chan readResult
	readBuffer   *bytes.Buffer
	readDeadline atomic.Value
	readNotify   chan struct{}
	readLock     sync.Mutex

	writeDeadline atomic.Value
	writeNotify   chan struct{}
	writeLock     sync.Mutex

	mu sync.RWMutex
	wg sync.WaitGroup

	die chan struct{}

	closed    uatomic.Bool
	closeOnce sync.Once
}

var _ net.Conn = (*Session)(nil)

func newSession(cfg sessionConfig) *Session {
	return &Session{
		listener:    cfg.listener,
		raddr:       cfg.raddr,
		readCh:      make(chan readResult, cfg.ReadBacklog),
		readBuffer:  bytes.NewBuffer(make([]byte, 0, cfg.InitialBufferSize)),
		readNotify:  make(chan struct{}),
		writeNotify: make(chan struct{}),
		die:         make(chan struct{}),
	}
}

func (s *Session) LocalAddr() net.Addr {
	return s.listener.Addr()
}

func (s *Session) RemoteAddr() net.Addr {
	return s.raddr
}

func (s *Session) Read(b []byte) (int, error) {
	if err := s.errIfClosed(); err != nil {
		return 0, err
	}
	if len(b) <= 0 {
		return 0, io.ErrShortBuffer
	}
	if err := s.listener.getReadError(); err != nil {
		return 0, err
	}
	s.readLock.Lock()
	defer s.readLock.Unlock()
	if s.readBuffer.Len() >= len(b) {
		return s.readBuffer.Read(b)
	}
	var deadline <-chan time.Time
	for {
		if t, ok := s.readDeadline.Load().(time.Time); ok && !t.IsZero() {
			timer := time.NewTimer(time.Until(t))
			defer timer.Stop()
			deadline = timer.C
		}
		select {
		case res := <-s.readCh:
			if res.err != nil {
				return 0, res.err
			}
			s.readBuffer.Write(res.data)
			return s.readBuffer.Read(b)
		case <-s.readNotify:
		case <-deadline:
			return 0, uerrors.ErrTimeout
		case <-s.die:
			return 0, io.EOF
		}
	}
}

func (s *Session) Write(b []byte) (int, error) {
	if err := s.errIfClosed(); err != nil {
		return 0, err
	}
	s.writeLock.Lock()
	defer s.writeLock.Unlock()
	var deadline <-chan time.Time
	ch := s.listener.writeAsync(s.raddr, b)
	for {
		if t, ok := s.writeDeadline.Load().(time.Time); ok && !t.IsZero() {
			timer := time.NewTimer(time.Until(t))
			defer timer.Stop()
			deadline = timer.C
		}
		select {
		case wr := <-ch:
			return wr.n, wr.err
		case <-s.writeNotify:
		case <-deadline:
			return 0, uerrors.ErrTimeout
		case <-s.die:
			return 0, io.EOF
		}
	}
}

func (s *Session) SetDeadline(t time.Time) error {
	if err := s.SetReadDeadline(t); err != nil {
		return err
	}
	if err := s.SetWriteDeadline(t); err != nil {
		return err
	}
	return nil
}

func (s *Session) SetReadDeadline(t time.Time) error {
	s.readDeadline.Store(t)
	util.AsyncNotify(s.readNotify)
	return nil
}

func (s *Session) SetWriteDeadline(t time.Time) error {
	s.writeDeadline.Store(t)
	util.AsyncNotify(s.writeNotify)
	return nil
}

func (s *Session) Close() error {
	return s.internalClose(true)
}

func (s *Session) dispatch(res readResult) {
	if s.closed.Get() {
		return
	}
	s.readCh <- res
}

func (s *Session) internalClose(remove bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.errIfClosed(); err != nil {
		return err
	}
	s.closed.Set(true)
	s.closeOnce.Do(func() {
		close(s.die)
	})
	if remove {
		s.listener.removeSession(s.raddr)
	}
	s.wg.Wait()
	return nil
}

func (s *Session) errIfClosed() error {
	if s.closed.Get() {
		return io.ErrClosedPipe
	}
	return nil
}
