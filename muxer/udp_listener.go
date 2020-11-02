package muxer

import (
	uatomic "datagram-toolkit/util/atomic"
	"errors"
	"io"
	"net"
	"sync"
)

const (
	defaultAcceptBacklog     = 5
	defaultInitialBufferSize = 4096
	defaultReadBufferSize    = 65535
	defaultReadBacklog       = 1024
	defaultWriteBacklog      = 1024

	minInitialBufferSize = 512
)

var ErrListenerClosed = errors.New("listener closed")

type UDPConfig struct {
	AcceptBacklog     int
	InitialBufferSize int
	ReadBufferSize    int
	ReadBacklog       int
	WriteBacklog      int
}

func DefaultConfig() UDPConfig {
	return UDPConfig{
		AcceptBacklog:     defaultAcceptBacklog,
		InitialBufferSize: defaultInitialBufferSize,
		ReadBufferSize:    defaultReadBufferSize,
		ReadBacklog:       defaultReadBacklog,
		WriteBacklog:      defaultWriteBacklog,
	}
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

	mu sync.RWMutex
	wg sync.WaitGroup

	accept chan *UDPSession
	write  chan udpWriteRequest
	die    chan struct{}

	closed uatomic.Bool
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
	if cfg.InitialBufferSize < minInitialBufferSize {
		cfg.InitialBufferSize = minInitialBufferSize
	}
	ul := &UDPListener{
		conn:     conn,
		cfg:      cfg,
		sessions: make(map[string]*UDPSession),
		accept:   make(chan *UDPSession, cfg.AcceptBacklog),
		write:    make(chan udpWriteRequest, cfg.WriteBacklog),
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
	if ul.closed.Get() {
		return nil, ErrListenerClosed
	}
	select {
	case us := <-ul.accept:
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
	if ul.closed.Get() {
		return nil, ErrListenerClosed
	}
	return ul.getSession(raddr), nil
}

func (ul *UDPListener) Close() error {
	ul.mu.Lock()
	defer ul.mu.Unlock()
	if ul.closed.Get() {
		return ErrListenerClosed
	}
	for _, us := range ul.sessions {
		if err := us.internalClose(false); err != nil {
			return err
		}
	}
	close(ul.die)
	if err := ul.conn.Close(); err != nil {
		return err
	}
	ul.wg.Wait()
	ul.closed.Set(true)
	return nil
}

func (ul *UDPListener) readRoutine() {
	defer ul.wg.Done()
	buf := make([]byte, ul.cfg.ReadBufferSize)
	for {
		n, raddr, err := ul.conn.ReadFromUDP(buf)
		if err != nil {
			if _, ok := err.(*net.OpError); !ok {
				log.Errorf("Read error: %+v", err)
			}
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
		case wr := <-ul.write:
			var res udpWriteResult
			res.n, res.err = ul.conn.WriteToUDP(wr.data, wr.raddr)
			wr.result <- res
			if res.err != nil {
				if _, ok := res.err.(*net.OpError); !ok {
					log.Errorf("Read error: %+v", res.err)
				}
				return
			}
		case <-ul.die:
			return
		}
	}
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
	ul.accept <- us
	ul.mu.Unlock()

	return us
}

func (ul *UDPListener) remove(raddr *net.UDPAddr) {
	ul.mu.Lock()
	defer ul.mu.Unlock()
	delete(ul.sessions, raddr.String())
}
