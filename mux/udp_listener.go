package mux

import (
	uatomic "datagram-toolkit/util/atomic"
	"io"
	"net"
	"sync"
	"sync/atomic"
)

type UDPConfig struct {
	AcceptBacklog     int
	InitialBufferSize int
	ReadBufferSize    int
	ReadBacklog       int
	WriteBacklog      int
}

func DefaultUDPConfig() UDPConfig {
	return UDPConfig{
		AcceptBacklog:     defaultAcceptBacklog,
		InitialBufferSize: defaultInitialBufferSize,
		ReadBufferSize:    defaultReadBufferSize,
		ReadBacklog:       defaultReadBacklog,
		WriteBacklog:      defaultWriteBacklog,
	}
}

func sanitizeUDPConfig(cfg UDPConfig) UDPConfig {
	if cfg.AcceptBacklog < 0 {
		cfg.AcceptBacklog = 0
	}
	if cfg.InitialBufferSize < minBufferSize {
		cfg.InitialBufferSize = minBufferSize
	}
	if cfg.ReadBufferSize < minBufferSize {
		cfg.InitialBufferSize = minBufferSize
	}
	if cfg.ReadBacklog < 0 {
		cfg.ReadBacklog = 0
	}
	if cfg.WriteBacklog < 0 {
		cfg.WriteBacklog = 0
	}
	return cfg
}

type udpWriteRequest struct {
	raddr  *net.UDPAddr
	data   []byte
	result chan<- udpWriteResult
}

type udpWriteResult struct {
	n   int
	err error
}

type UDPListener struct {
	conn *net.UDPConn
	cfg  UDPConfig

	sessions map[string]*UDPSession

	readErr atomic.Value

	acceptCh chan *UDPSession
	writeCh  chan udpWriteRequest

	mu sync.RWMutex
	wg sync.WaitGroup

	die chan struct{}

	closed    uatomic.Bool
	closeOnce sync.Once
}

var _ net.Listener = (*UDPListener)(nil)

func ListenUDP(network string, laddr *net.UDPAddr, cfg UDPConfig) (*UDPListener, error) {
	conn, err := net.ListenUDP(network, laddr)
	if err != nil {
		return nil, err
	}
	return NewUDPListener(conn, cfg), nil
}

func NewUDPListener(conn *net.UDPConn, cfg UDPConfig) *UDPListener {
	cfg = sanitizeUDPConfig(cfg)
	ul := &UDPListener{
		conn:     conn,
		cfg:      cfg,
		sessions: make(map[string]*UDPSession),
		acceptCh: make(chan *UDPSession, cfg.AcceptBacklog),
		writeCh:  make(chan udpWriteRequest, cfg.WriteBacklog),
		die:      make(chan struct{}),
	}
	ul.wg.Add(2)
	go ul.readRoutine()
	go ul.writeRoutine()
	return ul
}

func (ul *UDPListener) Addr() net.Addr {
	return ul.conn.LocalAddr()
}

func (ul *UDPListener) Accept() (net.Conn, error) {
	if err := ul.errIfClosed(); err != nil {
		return nil, err
	}
	select {
	case us := <-ul.acceptCh:
		return us, nil
	case <-ul.die:
		return nil, io.EOF
	}
}

func (ul *UDPListener) Dial(network string, addr string) (*UDPSession, error) {
	raddr, err := net.ResolveUDPAddr(network, addr)
	if err != nil {
		return nil, err
	}
	return ul.Open(raddr)
}

func (ul *UDPListener) Open(raddr *net.UDPAddr) (*UDPSession, error) {
	if err := ul.errIfClosed(); err != nil {
		return nil, err
	}
	return ul.getSession(raddr), nil
}

func (ul *UDPListener) Close() error {
	ul.mu.Lock()
	defer ul.mu.Unlock()
	if err := ul.errIfClosed(); err != nil {
		return err
	}
	for _, us := range ul.sessions {
		if err := us.internalClose(false); err != nil {
			return err
		}
	}
	ul.closeOnce.Do(func() {
		close(ul.die)
	})
	if err := ul.conn.Close(); err != nil {
		return err
	}
	ul.closed.Set(true)
	ul.wg.Wait()
	return nil
}

func (ul *UDPListener) readRoutine() {
	defer ul.wg.Done()
	buf := make([]byte, ul.cfg.ReadBufferSize)
	for {
		n, raddr, err := ul.conn.ReadFromUDP(buf)
		if err != nil {
			ul.readErr.Store(err)
			return
		}
		data := make([]byte, n)
		copy(data, buf[:n])
		ul.getSession(raddr).dispatch <- data
	}
}

func (ul *UDPListener) writeRoutine() {
	defer ul.wg.Done()
	for {
		select {
		case wr := <-ul.writeCh:
			var res udpWriteResult
			res.n, res.err = ul.conn.WriteToUDP(wr.data, wr.raddr)
			wr.result <- res
		case <-ul.die:
			return
		}
	}
}

func (ul *UDPListener) write(raddr *net.UDPAddr, b []byte) <-chan udpWriteResult {
	ch := make(chan udpWriteResult, 1)
	ul.writeCh <- udpWriteRequest{
		raddr:  raddr,
		data:   b,
		result: ch,
	}
	return ch
}

func (ul *UDPListener) getSession(raddr *net.UDPAddr) *UDPSession {
	addr := raddr.String()
	ul.mu.RLock()
	us, ok := ul.sessions[addr]
	ul.mu.RUnlock()
	if ok {
		return us
	}

	us = &UDPSession{
		listener: ul,
		conn:     ul.conn,
		raddr:    raddr,
	}
	us.init(ul.cfg)
	ul.mu.Lock()
	ul.sessions[addr] = us
	ul.acceptCh <- us
	ul.mu.Unlock()

	return us
}

func (ul *UDPListener) removeSession(raddr *net.UDPAddr) {
	ul.mu.Lock()
	defer ul.mu.Unlock()
	delete(ul.sessions, raddr.String())
}

func (ul *UDPListener) getReadError() error {
	if err, ok := ul.readErr.Load().(error); ok {
		return err
	}
	return nil
}

func (ul *UDPListener) errIfClosed() error {
	if ul.closed.Get() {
		return io.ErrClosedPipe
	}
	return nil
}
