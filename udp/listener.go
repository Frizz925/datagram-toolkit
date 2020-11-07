package udp

import (
	uatomic "datagram-toolkit/util/atomic"
	"io"
	"net"
	"sync"
	"sync/atomic"
)

type writeRequest struct {
	raddr  *net.UDPAddr
	data   []byte
	result chan<- writeResult
}

type writeResult struct {
	n   int
	err error
}

type readResult struct {
	data []byte
	err  error
}

type Listener struct {
	conn *net.UDPConn
	cfg  Config

	sessions map[string]*Session

	readErr   atomic.Value
	readErrCh chan error

	acceptCh chan *Session
	writeCh  chan writeRequest

	mu sync.RWMutex
	wg sync.WaitGroup

	die chan struct{}

	closed    uatomic.Bool
	closeOnce sync.Once
}

var _ net.Listener = (*Listener)(nil)

func Listen(network string, laddr *net.UDPAddr, cfg Config) (*Listener, error) {
	conn, err := net.ListenUDP(network, laddr)
	if err != nil {
		return nil, err
	}
	return NewListener(conn, cfg), nil
}

func NewListener(conn *net.UDPConn, cfg Config) *Listener {
	cfg = sanitizeConfig(cfg)
	l := &Listener{
		conn:     conn,
		cfg:      cfg,
		sessions: make(map[string]*Session),
		acceptCh: make(chan *Session, cfg.AcceptBacklog),
		writeCh:  make(chan writeRequest, cfg.WriteBacklog),
		die:      make(chan struct{}),
	}
	l.wg.Add(2)
	go l.readRoutine()
	go l.writeRoutine()
	return l
}

func (l *Listener) Addr() net.Addr {
	return l.conn.LocalAddr()
}

func (l *Listener) Accept() (net.Conn, error) {
	if err := l.errIfClosed(); err != nil {
		return nil, err
	}
	select {
	case us := <-l.acceptCh:
		return us, nil
	case <-l.die:
		return nil, io.EOF
	}
}

func (l *Listener) Dial(network string, addr string) (*Session, error) {
	raddr, err := net.ResolveUDPAddr(network, addr)
	if err != nil {
		return nil, err
	}
	return l.Open(raddr)
}

func (l *Listener) Open(raddr *net.UDPAddr) (*Session, error) {
	if err := l.errIfClosed(); err != nil {
		return nil, err
	}
	return l.getSession(raddr), nil
}

func (l *Listener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.errIfClosed(); err != nil {
		return err
	}
	for _, us := range l.sessions {
		if err := us.internalClose(false); err != nil {
			return err
		}
	}
	l.closeOnce.Do(func() {
		close(l.die)
	})
	if err := l.conn.Close(); err != nil {
		return err
	}
	l.closed.Set(true)
	l.wg.Wait()
	return nil
}

func (l *Listener) readRoutine() {
	defer l.wg.Done()
	buf := make([]byte, l.cfg.ReadBufferSize)
	for {
		n, raddr, err := l.conn.ReadFromUDP(buf)
		if err != nil {
			go l.handleReadError(err)
			return
		}
		var res readResult
		res.data = make([]byte, n)
		copy(res.data, buf)
		l.getSession(raddr).dispatch(res)
	}
}

func (l *Listener) writeRoutine() {
	defer l.wg.Done()
	for {
		select {
		case wr := <-l.writeCh:
			var res writeResult
			res.n, res.err = l.conn.WriteToUDP(wr.data, wr.raddr)
			wr.result <- res
		case <-l.die:
			return
		}
	}
}

func (l *Listener) handleReadError(err error) {
	l.readErr.Store(err)
	l.readErrCh <- err

	l.mu.RLock()
	res := readResult{err: err}
	for _, s := range l.sessions {
		s.dispatch(res)
	}
	l.mu.RUnlock()
}

func (l *Listener) writeAsync(raddr *net.UDPAddr, b []byte) <-chan writeResult {
	ch := make(chan writeResult, 1)
	l.writeCh <- writeRequest{
		raddr:  raddr,
		data:   b,
		result: ch,
	}
	return ch
}

func (l *Listener) getSession(raddr *net.UDPAddr) *Session {
	addr := raddr.String()

	l.mu.RLock()
	s, ok := l.sessions[addr]
	l.mu.RUnlock()

	if !ok {
		s = newSession(sessionConfig{
			Config:   l.cfg,
			listener: l,
			raddr:    raddr,
		})
		l.mu.Lock()
		l.sessions[addr] = s
		l.mu.Unlock()
		l.acceptCh <- s
	}

	return s
}

func (l *Listener) removeSession(raddr *net.UDPAddr) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.sessions, raddr.String())
}

func (l *Listener) getReadError() error {
	if err, ok := l.readErr.Load().(error); ok {
		return err
	}
	return nil
}

func (l *Listener) errIfClosed() error {
	if l.closed.Get() {
		return io.ErrClosedPipe
	}
	return nil
}
