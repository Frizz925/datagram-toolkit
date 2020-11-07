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

	readCh   chan readResult
	readBuf  *bytes.Buffer
	readLock sync.Mutex

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
	return &IOStream{
		muxer:   cfg.muxer,
		id:      cfg.id,
		readCh:  make(chan readResult, cfg.readBacklog),
		readBuf: bytes.NewBuffer(make([]byte, 0, cfg.initialBufferSize)),
		die:     make(chan struct{}),
	}
}

func (ios *IOStream) ID() uint32 {
	return ios.id
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
	if ios.readBuf.Len() >= len(b) {
		return ios.readBuf.Read(b)
	}
	select {
	case res := <-ios.readCh:
		if res.err != nil {
			return 0, res.err
		}
		ios.readBuf.Write(res.data)
		ios.muxer.readBuffers.Put(res.head)
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

func (ios *IOStream) dispatch(res readResult) {
	if ios.closed.Get() {
		return
	}
	ios.readCh <- res
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
	return nil
}

func (ios *IOStream) errIfClosed() error {
	if ios.closed.Get() {
		return io.ErrClosedPipe
	}
	return nil
}
