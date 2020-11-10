package mocks

import (
	"bytes"
	"datagram-toolkit/util"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type readResult struct {
	data []byte
	err  error
}

type writeRequest struct {
	data   []byte
	result chan<- writeResult
}

type writeResult struct {
	n   int
	err error
}

type conn struct {
	reader *io.PipeReader
	writer *io.PipeWriter

	rawBuffer  []byte
	buffer     *bytes.Buffer
	bufferLock sync.Mutex

	readQueue    chan readResult
	readNotify   chan struct{}
	readDeadline atomic.Value
	readError    atomic.Value

	writeQueue    chan writeRequest
	writeNotify   chan struct{}
	writeDeadline atomic.Value

	die chan struct{}
	wg  sync.WaitGroup

	closeOnce sync.Once
}

func Conn() (net.Conn, net.Conn) {
	pr1, pw1 := io.Pipe()
	pr2, pw2 := io.Pipe()
	c1 := NewConn(pr1, pw2)
	c2 := NewConn(pr2, pw1)
	return c1, c2
}

func NewConn(pr *io.PipeReader, pw *io.PipeWriter) net.Conn {
	c := &conn{
		reader:      pr,
		writer:      pw,
		rawBuffer:   make([]byte, 512),
		buffer:      bytes.NewBuffer(make([]byte, 0, 512)),
		readQueue:   make(chan readResult, 512),
		readNotify:  make(chan struct{}),
		writeQueue:  make(chan writeRequest, 512),
		writeNotify: make(chan struct{}),
		die:         make(chan struct{}),
	}
	c.wg.Add(2)
	go c.readRoutine()
	go c.writeRoutine()
	return c
}

func (c *conn) Read(b []byte) (int, error) {
	if len(b) <= 0 {
		return 0, io.ErrShortBuffer
	}
	if err, ok := c.readError.Load().(error); ok {
		return 0, err
	}
	c.bufferLock.Lock()
	defer c.bufferLock.Unlock()
	var deadline <-chan time.Time
	for {
		if c.buffer.Len() > 0 {
			return c.buffer.Read(b)
		}
		if t, ok := c.readDeadline.Load().(time.Time); ok && !t.IsZero() {
			timer := time.NewTimer(time.Until(t))
			defer timer.Stop()
			deadline = timer.C
		}
		select {
		case res := <-c.readQueue:
			if res.err != nil {
				return 0, res.err
			}
			if len(res.data) <= 0 {
				return 0, io.EOF
			}
			c.buffer.Write(res.data)
		case <-c.readNotify:
		case <-deadline:
			return 0, os.ErrDeadlineExceeded
		case <-c.die:
			return 0, io.ErrClosedPipe
		}
	}
}

func (c *conn) Write(b []byte) (int, error) {
	if len(b) <= 0 {
		return 0, io.EOF
	}
	ch := make(chan writeResult, 1)
	c.writeQueue <- writeRequest{b, ch}
	var deadline <-chan time.Time
	for {
		if t, ok := c.writeDeadline.Load().(time.Time); ok && !t.IsZero() {
			timer := time.NewTimer(time.Until(t))
			defer timer.Stop()
			deadline = timer.C
		}
		select {
		case res := <-ch:
			return res.n, res.err
		case <-c.writeNotify:
		case <-deadline:
			return 0, os.ErrDeadlineExceeded
		case <-c.die:
			return 0, io.ErrClosedPipe
		}
	}
}

func (c *conn) LocalAddr() net.Addr {
	return nil
}

func (c *conn) RemoteAddr() net.Addr {
	return nil
}

func (c *conn) SetReadDeadline(t time.Time) error {
	c.readDeadline.Store(t)
	util.AsyncNotify(c.readNotify)
	return nil
}

func (c *conn) SetWriteDeadline(t time.Time) error {
	c.readDeadline.Store(t)
	util.AsyncNotify(c.writeNotify)
	return nil
}

func (c *conn) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}
	if err := c.SetWriteDeadline(t); err != nil {
		return err
	}
	return nil
}

func (c *conn) Close() error {
	c.closeOnce.Do(func() {
		close(c.die)
	})
	if err := c.reader.Close(); err != nil {
		return err
	}
	if err := c.writer.Close(); err != nil {
		return err
	}
	for draining := true; draining; {
		select {
		case <-c.readQueue:
		case <-c.writeQueue:
		default:
			draining = false
		}
	}
	c.wg.Wait()
	return nil
}

func (c *conn) readRoutine() {
	defer c.wg.Done()
	for {
		n, err := c.reader.Read(c.rawBuffer)
		if err != nil {
			c.readError.Store(err)
			c.readQueue <- readResult{err: err}
			return
		}
		data := make([]byte, n)
		copy(data, c.rawBuffer)
		c.readQueue <- readResult{data: data}
	}
}

func (c *conn) writeRoutine() {
	defer c.wg.Done()
	for {
		select {
		case req := <-c.writeQueue:
			var res writeResult
			res.n, res.err = c.writer.Write(req.data)
			req.result <- res
		case <-c.die:
			return
		}
	}
}
