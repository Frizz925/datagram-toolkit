package mux

import (
	"bytes"
	uatomic "datagram-toolkit/util/atomic"
	"io"
	"sync"
)

type IOStream struct {
	muxer *IOMuxer
	id    uint32

	readCh   chan []byte
	readBuf  *bytes.Buffer
	readLock sync.Mutex

	wg sync.WaitGroup
	mu sync.Mutex

	die chan struct{}

	closed    uatomic.Bool
	closeOnce sync.Once
}

type ioStreamConfig struct {
	muxer             *IOMuxer
	id                uint32
	initialBufferSize int
	readBacklog       int
}

var _ Stream = (*IOStream)(nil)

func newIOStream(cfg ioStreamConfig) *IOStream {
	ios := &IOStream{
		muxer:   cfg.muxer,
		id:      cfg.id,
		readCh:  make(chan []byte, cfg.readBacklog),
		readBuf: bytes.NewBuffer(make([]byte, 0, cfg.initialBufferSize)),
		die:     make(chan struct{}),
	}
	ios.wg.Add(1)
	go ios.readRoutine()
	return ios
}

func (ios *IOStream) Read(b []byte) (int, error) {
	if err := ios.errIfClosed(); err != nil {
		return 0, err
	}
	if len(b) <= 0 {
		return 0, io.ErrShortBuffer
	}
	if err := ios.muxer.getReadError(); err != nil {
		return 0, err
	}
	ios.readLock.Lock()
	defer ios.readLock.Unlock()
	if ios.readBuf.Len() > len(b) {
		return ios.readBuf.Read(b)
	}
	select {
	case r := <-ios.readCh:
		ios.readBuf.Write(r)
		return ios.readBuf.Read(b)
	case <-ios.die:
		return 0, io.EOF
	}
}

func (ios *IOStream) Write(b []byte) (int, error) {
	if err := ios.errIfClosed(); err != nil {
		return 0, err
	}
	return ios.muxer.write(ios.id, cmdSTD, b)
}

func (ios *IOStream) Close() error {
	return ios.close(true)
}

func (ios *IOStream) readRoutine() {
	defer ios.wg.Done()
}

func (ios *IOStream) dispatch(b []byte) {
	if ios.closed.Get() {
		return
	}
	ios.readCh <- b
}

func (ios *IOStream) close(remove bool) error {
	ios.mu.Lock()
	defer ios.mu.Unlock()
	if err := ios.errIfClosed(); err != nil {
		return err
	}
	ios.closed.Set(true)
	ios.closeOnce.Do(func() {
		close(ios.die)
	})
	if remove {
		ios.muxer.removeStream(ios.id)
	}
	if _, err := ios.muxer.write(ios.id, cmdFIN, nil); err != nil {
		return err
	}
	ios.wg.Wait()
	return nil
}

func (ios *IOStream) errIfClosed() error {
	if ios.closed.Get() {
		return io.ErrClosedPipe
	}
	return nil
}
