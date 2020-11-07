package mux

import (
	"datagram-toolkit/util"
	uatomic "datagram-toolkit/util/atomic"
	"io"
	"math"
	"sync"
	"sync/atomic"
)

type IOMuxer struct {
	rwc io.ReadWriteCloser
	cfg Config
	idg util.IDGenerator

	streams map[uint32]*IOStream

	acceptCh   chan *IOStream
	acceptLock sync.Mutex

	readErr atomic.Value

	writeCh chan writeRequest

	mu sync.RWMutex
	wg sync.WaitGroup

	die chan struct{}

	closed uatomic.Bool
}

var _ Muxer = (*IOMuxer)(nil)

func NewIOMuxer(rwc io.ReadWriteCloser, cfg Config) *IOMuxer {
	cfg = sanitizeConfig(cfg)
	iom := &IOMuxer{
		rwc:      rwc,
		cfg:      cfg,
		streams:  make(map[uint32]*IOStream),
		acceptCh: make(chan *IOStream, cfg.AcceptBacklog),
		writeCh:  make(chan writeRequest, cfg.WriteBacklog),
	}
	iom.wg.Add(2)
	go iom.readRoutine()
	go iom.writeRoutine()
	return iom
}

func (iom *IOMuxer) Open() (io.ReadWriteCloser, error) {
	return iom.OpenStream()
}

func (iom *IOMuxer) OpenStream() (Stream, error) {
	if err := iom.errIfClosed(); err != nil {
		return nil, err
	}

	// Find the next available stream ID
	id := iom.idg.Next()
	iom.mu.RLock()
	for {
		_, ok := iom.streams[id]
		if !ok {
			break
		}
		if id >= math.MaxUint32-1 {
			return nil, ErrStreamExhausted
		}
		id = iom.idg.Next()
	}
	iom.mu.RUnlock()

	// Check for read error before sending down the wire
	if err := iom.getReadError(); err != nil {
		return nil, err
	}

	// Send the SYN frame
	_, err := iom.write(id, cmdSYN, nil)
	if err != nil {
		return nil, err
	}

	// Wait for the incoming SYN frame via accept channel
	iom.acceptLock.Lock()
	defer iom.acceptLock.Unlock()
	select {
	case ios := <-iom.acceptCh:
		return ios, nil
	case <-iom.die:
		return nil, io.EOF
	}
}

func (iom *IOMuxer) Accept() (io.ReadWriteCloser, error) {
	return iom.AcceptStream()
}

func (iom *IOMuxer) AcceptStream() (Stream, error) {
	if err := iom.errIfClosed(); err != nil {
		return nil, err
	}
	if err := iom.getReadError(); err != nil {
		return nil, err
	}
	iom.acceptLock.Lock()
	defer iom.acceptLock.Unlock()
	select {
	case ios := <-iom.acceptCh:
		// Reply to the incoming ACK frame
		_, err := iom.write(ios.id, cmdSYN, nil)
		return ios, err
	case <-iom.die:
		return nil, io.EOF
	}
}

func (iom *IOMuxer) Close() error {
	return nil
}

func (iom *IOMuxer) readRoutine() {
	defer iom.wg.Done()
	buf := make([]byte, iom.cfg.ReadBufferSize)
	for {
		n, err := iom.rwc.Read(buf)
		if err != nil {
			return
		}
		if n < headerSize {
			continue
		}
		f := frame(buf[:n])
		id := f.StreamID()
		ios := iom.getStream(id)
		switch f.Cmd() {
		case cmdSYN:
			iom.acceptStream(ios)
		case cmdSTD:
			iom.dispatchStream(ios, f)
		case cmdFIN:
			iom.closeStream(ios)
		}
	}
}

func (iom *IOMuxer) writeRoutine() {
	defer iom.wg.Done()
	for {
		select {
		case req := <-iom.writeCh:
			var res writeResult
			n, err := iom.rwc.Write(req.frame)
			if err != nil {
				res.err = err
			} else {
				res.n = n - headerSize
			}
			req.result <- res
		case <-iom.die:
			return
		}
	}
}

func (iom *IOMuxer) write(id uint32, cmd uint8, b []byte) (int, error) {
	res := <-iom.writeAsync(id, cmd, b)
	return res.n, res.err
}

func (iom *IOMuxer) writeAsync(id uint32, cmd uint8, b []byte) <-chan writeResult {
	ch := make(chan writeResult, 1)
	iom.writeCh <- writeRequest{
		frame:  newFrame(id, cmd, b),
		result: ch,
	}
	return ch
}

func (iom *IOMuxer) getStream(id uint32) *IOStream {
	iom.mu.RLock()
	ios, ok := iom.streams[id]
	iom.mu.RUnlock()
	if !ok {
		ios = iom.openStream(id)
	}
	return ios
}

func (iom *IOMuxer) openStream(id uint32) *IOStream {
	ios := newIOStream(ioStreamConfig{
		muxer:       iom,
		id:          id,
		readBacklog: iom.cfg.ReadBacklog,
	})
	iom.mu.Lock()
	iom.streams[id] = ios
	iom.mu.Unlock()
	return ios
}

func (iom *IOMuxer) removeStream(id uint32) {
	iom.mu.Lock()
	defer iom.mu.Unlock()
	delete(iom.streams, id)
}

func (iom *IOMuxer) acceptStream(ios *IOStream) {
	iom.acceptCh <- ios
}

func (iom *IOMuxer) dispatchStream(ios *IOStream, f frame) {
	b := make([]byte, f.Len())
	copy(b, f.Body())
	ios.dispatch(b)
}

func (iom *IOMuxer) closeStream(ios *IOStream) {
	//nolint:errcheck
	ios.Close()
}

func (iom *IOMuxer) getReadError() error {
	if err, ok := iom.readErr.Load().(error); ok {
		return err
	}
	return nil
}

func (iom *IOMuxer) errIfClosed() error {
	if iom.closed.Get() {
		return io.ErrClosedPipe
	}
	return nil
}
