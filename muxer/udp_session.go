package muxer

import (
	"bytes"
	"datagram-toolkit/util"
	uatomic "datagram-toolkit/util/atomic"
	uerrors "datagram-toolkit/util/errors"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var ErrSessionClosed = errors.New("session closed")

type UDPSession struct {
	listener *UDPListener
	conn     *net.UDPConn
	raddr    *net.UDPAddr

	buffer *bytes.Buffer

	mu sync.RWMutex
	wg sync.WaitGroup

	readDeadline  atomic.Value
	writeDeadline atomic.Value

	dispatch    chan []byte
	readNotify  chan struct{}
	writeNotify chan struct{}
	die         chan struct{}

	closed uatomic.Bool
}

var _ net.Conn = (*UDPSession)(nil)

func (us *UDPSession) init(cfg UDPConfig) {
	us.mu.Lock()
	defer us.mu.Unlock()
	us.buffer = bytes.NewBuffer(make([]byte, 0, cfg.InitialBufferSize))
	us.dispatch = make(chan []byte, cfg.ReadBacklog)
	us.readNotify = make(chan struct{})
	us.writeNotify = make(chan struct{})
	us.die = make(chan struct{})

	us.wg.Add(1)
	go us.dispatchRoutine()
}

func (us *UDPSession) LocalAddr() net.Addr {
	return us.conn.LocalAddr()
}

func (us *UDPSession) RemoteAddr() net.Addr {
	return us.raddr
}

func (us *UDPSession) Read(b []byte) (int, error) {
	if us.closed.Get() {
		return 0, ErrSessionClosed
	}
	var deadline <-chan time.Time
	for {
		if us.bufferLen() > 0 {
			us.mu.Lock()
			defer us.mu.Unlock()
			return us.buffer.Read(b)
		}
		if t, ok := us.readDeadline.Load().(time.Time); ok && !t.IsZero() {
			timer := time.NewTimer(time.Until(t))
			defer timer.Stop()
			deadline = timer.C
		}
		select {
		case <-us.readNotify:
		case <-deadline:
			return 0, uerrors.ErrTimeout
		case <-us.die:
			return 0, io.EOF
		}
	}
}

func (us *UDPSession) Write(b []byte) (int, error) {
	if us.closed.Get() {
		return 0, ErrSessionClosed
	}
	var deadline <-chan time.Time
	ch := make(chan udpWriteResult)
	us.listener.write <- udpWriteRequest{
		raddr:  us.raddr,
		data:   b,
		result: ch,
	}
	for {
		if t, ok := us.writeDeadline.Load().(time.Time); ok && !t.IsZero() {
			timer := time.NewTimer(time.Until(t))
			defer timer.Stop()
			deadline = timer.C
		}
		select {
		case wr := <-ch:
			if wr.err != nil {
				return 0, wr.err
			}
			return wr.n, nil
		case <-us.writeNotify:
		case <-deadline:
			return 0, uerrors.ErrTimeout
		case <-us.die:
			return 0, io.EOF
		}
	}
}

func (us *UDPSession) SetDeadline(t time.Time) error {
	if err := us.SetReadDeadline(t); err != nil {
		return err
	}
	if err := us.SetWriteDeadline(t); err != nil {
		return err
	}
	return nil
}

func (us *UDPSession) SetReadDeadline(t time.Time) error {
	us.readDeadline.Store(t)
	util.AsyncNotify(us.readNotify)
	return nil
}

func (us *UDPSession) SetWriteDeadline(t time.Time) error {
	us.writeDeadline.Store(t)
	util.AsyncNotify(us.writeNotify)
	return nil
}

func (us *UDPSession) Close() error {
	return us.internalClose(true)
}

func (us *UDPSession) dispatchRoutine() {
	defer us.wg.Done()
	for {
		select {
		case b := <-us.dispatch:
			us.mu.Lock()
			us.buffer.Write(b)
			us.mu.Unlock()
			util.AsyncNotify(us.readNotify)
		case <-us.die:
			return
		}
	}
}

func (us *UDPSession) bufferLen() int {
	us.mu.RLock()
	defer us.mu.RUnlock()
	return us.buffer.Len()
}

func (us *UDPSession) internalClose(remove bool) error {
	us.mu.Lock()
	defer us.mu.Unlock()
	if us.closed.Get() {
		return ErrSessionClosed
	}
	if remove {
		us.listener.remove(us.raddr)
	}
	close(us.die)
	us.wg.Wait()
	us.closed.Set(true)
	return nil
}
