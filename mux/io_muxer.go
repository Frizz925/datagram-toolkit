package mux

import (
	"datagram-toolkit/util"
	uatomic "datagram-toolkit/util/atomic"
	"io"
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

	readErr   atomic.Value
	readErrCh chan error

	writeCh chan writeRequest

	mu sync.RWMutex
	wg sync.WaitGroup

	die chan struct{}

	closed    uatomic.Bool
	closeOnce sync.Once
}

var _ Muxer = (*IOMuxer)(nil)

var handlerMap = map[uint8]func(*IOMuxer, frame){
	cmdSYN: (*IOMuxer).handleSYN,
	cmdSTD: (*IOMuxer).handleSTD,
	cmdFIN: (*IOMuxer).handleFIN,
}

func NewIOMuxer(rwc io.ReadWriteCloser, cfg Config) *IOMuxer {
	cfg = sanitizeConfig(cfg)
	iom := &IOMuxer{
		rwc:       rwc,
		cfg:       cfg,
		streams:   make(map[uint32]*IOStream),
		acceptCh:  make(chan *IOStream, cfg.AcceptBacklog),
		readErrCh: make(chan error, 1),
		writeCh:   make(chan writeRequest, cfg.WriteBacklog),
		die:       make(chan struct{}),
	}
	iom.wg.Add(2)
	go iom.readRoutine()
	go iom.writeRoutine()
	return iom
}

func (iom *IOMuxer) Open() (Stream, error) {
	if err := iom.errIfClosed(); err != nil {
		return nil, err
	}

	// Find the next available stream ID
	id := iom.idg.Next()
	iom.mu.RLock()
	for {
		// Zero-value ID is reserved for connection-level commands
		if id <= 0 {
			id = iom.idg.Next()
		}
		_, ok := iom.streams[id]
		if !ok {
			break
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
	case err := <-iom.readErrCh:
		return nil, err
	case <-iom.die:
		return nil, io.EOF
	}
}

func (iom *IOMuxer) Accept() (Stream, error) {
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
	case err := <-iom.readErrCh:
		return nil, err
	case <-iom.die:
		return nil, io.EOF
	}
}

func (iom *IOMuxer) Close() error {
	if err := iom.errIfClosed(); err != nil {
		return err
	}
	iom.mu.Lock()
	defer iom.mu.Unlock()
	for _, s := range iom.streams {
		if err := s.close(false); err != nil {
			return err
		}
	}
	iom.closed.Set(true)
	iom.closeOnce.Do(func() {
		close(iom.die)
	})
	if err := iom.rwc.Close(); err != nil {
		return err
	}
	iom.wg.Wait()
	return nil
}

func (iom *IOMuxer) readRoutine() {
	buf := make([]byte, iom.cfg.ReadBufferSize)
	for {
		n, err := iom.rwc.Read(buf)
		if err != nil {
			iom.wg.Done()
			iom.handleReadError(err)
			return
		}
		if n < headerSize {
			continue
		}
		f := frame(buf[:n])
		h := handlerMap[f.Cmd()]
		if h != nil {
			h(iom, f)
		}
	}
}

func (iom *IOMuxer) writeRoutine() {
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
			iom.wg.Done()
			return
		}
	}
}

func (iom *IOMuxer) handleReadError(err error) {
	iom.readErr.Store(err)
	iom.readErrCh <- err

	iom.mu.RLock()
	res := readResult{err: err}
	for _, ios := range iom.streams {
		ios.dispatch(res)
	}
	iom.mu.RUnlock()
}

func (iom *IOMuxer) handleSYN(f frame) {
	id := f.StreamID()
	if iom.hasStream(id) {
		return
	}
	ios := iom.openStream(id)
	iom.acceptCh <- ios
}

func (iom *IOMuxer) handleSTD(f frame) {
	ios := iom.getStream(f.StreamID())
	if ios == nil {
		return
	}
	b := make([]byte, f.Len())
	copy(b, f.Body())
	ios.dispatch(readResult{data: b})
}

func (iom *IOMuxer) handleFIN(f frame) {
	id := f.StreamID()
	if id > 0 {
		ios := iom.getStream(id)
		if ios != nil {
			//nolint:errcheck
			go ios.Close()
		}
	} else {
		//nolint:errcheck
		go iom.Close()
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

func (iom *IOMuxer) openStream(id uint32) *IOStream {
	ios := iom.getStream(id)
	if ios != nil {
		return ios
	}
	ios = newIOStream(ioStreamConfig{
		muxer:       iom,
		id:          id,
		readBacklog: iom.cfg.ReadBacklog,
	})
	iom.mu.Lock()
	iom.streams[id] = ios
	iom.mu.Unlock()
	return ios
}

func (iom *IOMuxer) getStream(id uint32) *IOStream {
	iom.mu.RLock()
	defer iom.mu.RUnlock()
	return iom.streams[id]
}

func (iom *IOMuxer) hasStream(id uint32) bool {
	iom.mu.RLock()
	defer iom.mu.RUnlock()
	_, ok := iom.streams[id]
	return ok
}

func (iom *IOMuxer) removeStream(id uint32) {
	iom.mu.Lock()
	defer iom.mu.Unlock()
	delete(iom.streams, id)
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
