package mux

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

	closed    uatomic.Bool
	closeOnce sync.Once
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
	if err := us.errIfClosed(); err != nil {
		return 0, err
	}
	if len(b) <= 0 {
		return 0, io.ErrShortBuffer
	}
	if err := us.listener.getReadError(); err != nil {
		return 0, err
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
	if err := us.errIfClosed(); err != nil {
		return 0, err
	}
	var deadline <-chan time.Time
	ch := us.listener.write(us.raddr, b)
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
	if err := us.errIfClosed(); err != nil {
		return err
	}
	us.closed.Set(true)
	us.closeOnce.Do(func() {
		close(us.die)
	})
	if remove {
		us.listener.removeSession(us.raddr)
	}
	us.wg.Wait()
	return nil
}

func (us *UDPSession) errIfClosed() error {
	if us.closed.Get() {
		return io.ErrClosedPipe
	}
	return nil
}
