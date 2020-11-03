package netem

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

	"github.com/sirupsen/logrus"
)

const (
	minBufferSize = 512
	minBacklog    = 1
)

var ErrNetemClosed = errors.New("netem closed")

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

type Config struct {
	// The size of the internal buffer
	BufferSize int
	// Number of packets can be kept in queue
	Backlog int

	// The size of emulated packet fragmentations on read.
	// Zero value means no emulation of packet fragmentations.
	ReadFragmentSize int
	// Packet at every nth would be discarded on read to emulate packet loss.
	// Zero value means no emulation of packet loss.
	ReadLossNth int
	// Packet at every nth would be duplicated on read to emulate packet duplication.
	// Zero value means no emulation of packet duplication.
	ReadDuplicateNth int
	// Packet at every nth would be reordered on read to emulate packet reordering.
	// Zero value means no emulation of packet reordering.
	ReadReorderNth int

	// The size of emulated packet fragmentations on write.
	// Zero value means no emulation of packet fragmentations.
	WriteFragmentSize int
	// Packet at every nth would be discarded on write to emulate packet loss.
	// Zero value means no emulation of packet loss.
	WriteLossNth int
	// Packet at every nth would be duplicated on write to emulate packet duplication.
	// Zero value means no emulation of packet duplication.
	WriteDuplicateNth int
	// Packet at every nth would be reordered on write to emulate packet reordering.
	// Zero value means no emulation of packet reordering.
	WriteReorderNth int
}

func DefaultConfig() Config {
	return Config{
		BufferSize: 4096,
		Backlog:    5,
	}
}

type Netem struct {
	net.Conn

	readLossNth      uint32
	readFragmentSize uint32
	readDuplicateNth uint32
	readReorderNth   uint32

	writeFragmentSize uint32
	writeLossNth      uint32
	writeDuplicateNth uint32
	writeReorderNth   uint32

	rawBuffer  []byte
	buffer     *bytes.Buffer
	bufferLock sync.Mutex

	readQueue    chan readResult
	readNotify   chan struct{}
	readCounter  uint32
	readDeadline atomic.Value
	readError    atomic.Value

	writeQueue    chan writeRequest
	writeNotify   chan struct{}
	writeCounter  uint32
	writeDeadline atomic.Value

	die chan struct{}

	wg sync.WaitGroup
	mu sync.Mutex

	closed uatomic.Bool
}

func New(conn net.Conn, cfg Config) *Netem {
	if cfg.BufferSize < minBufferSize {
		cfg.BufferSize = minBufferSize
	}
	if cfg.Backlog < minBacklog {
		cfg.Backlog = minBacklog
	}
	ne := &Netem{
		Conn: conn,

		rawBuffer: make([]byte, cfg.BufferSize),
		buffer:    bytes.NewBuffer(make([]byte, 0, cfg.BufferSize)),

		readQueue:  make(chan readResult, cfg.Backlog),
		readNotify: make(chan struct{}),

		writeQueue:  make(chan writeRequest, cfg.Backlog),
		writeNotify: make(chan struct{}),

		die: make(chan struct{}),
	}
	ne.Update(cfg)
	ne.wg.Add(2)
	go ne.readRoutine()
	go ne.writeRoutine()
	return ne
}

func (ne *Netem) Read(b []byte) (int, error) {
	if ne.closed.Get() {
		return 0, ErrNetemClosed
	}
	if len(b) <= 0 {
		return 0, io.EOF
	}
	if err, ok := ne.readError.Load().(error); ok {
		return 0, err
	}
	ne.bufferLock.Lock()
	defer ne.bufferLock.Unlock()
	var deadline <-chan time.Time
	for {
		if ne.buffer.Len() > 0 {
			n, err := ne.internalRead(b)
			if err != io.EOF {
				return n, err
			}
		}
		if t, ok := ne.readDeadline.Load().(time.Time); ok && !t.IsZero() {
			timer := time.NewTimer(time.Until(t))
			defer timer.Stop()
			deadline = timer.C
		}
		select {
		case res := <-ne.readQueue:
			if res.err != nil {
				return 0, res.err
			}
			ne.buffer.Write(res.data)
		case <-ne.readNotify:
		case <-deadline:
			return 0, uerrors.ErrTimeout
		case <-ne.die:
			return 0, io.EOF
		}
	}
}

func (ne *Netem) Write(b []byte) (int, error) {
	if ne.closed.Get() {
		return 0, ErrNetemClosed
	}
	if len(b) <= 0 {
		return 0, io.EOF
	}
	ch := make(chan writeResult, 1)
	ne.writeQueue <- writeRequest{b, ch}
	var deadline <-chan time.Time
	for {
		if t, ok := ne.writeDeadline.Load().(time.Time); ok && !t.IsZero() {
			timer := time.NewTimer(time.Until(t))
			defer timer.Stop()
			deadline = timer.C
		}
		select {
		case res := <-ch:
			return res.n, res.err
		case <-ne.writeNotify:
		case <-deadline:
			return 0, uerrors.ErrTimeout
		case <-ne.die:
			return 0, io.EOF
		}
	}
}

func (ne *Netem) SetDeadline(t time.Time) error {
	if err := ne.SetReadDeadline(t); err != nil {
		return err
	}
	if err := ne.SetWriteDeadline(t); err != nil {
		return err
	}
	return nil
}

func (ne *Netem) SetReadDeadline(t time.Time) error {
	ne.readDeadline.Store(t)
	util.AsyncNotify(ne.readNotify)
	return nil
}

func (ne *Netem) SetWriteDeadline(t time.Time) error {
	ne.writeDeadline.Store(t)
	util.AsyncNotify(ne.writeNotify)
	return nil
}

// Update the config for network emulation.
// May take effect on the next read/write operations.
func (ne *Netem) Update(cfg Config) {
	atomic.StoreUint32(&ne.readFragmentSize, uint32(cfg.ReadFragmentSize))
	atomic.StoreUint32(&ne.writeFragmentSize, uint32(cfg.WriteFragmentSize))
	atomic.StoreUint32(&ne.readLossNth, uint32(cfg.ReadLossNth))
	atomic.StoreUint32(&ne.writeLossNth, uint32(cfg.WriteLossNth))
	atomic.StoreUint32(&ne.readDuplicateNth, uint32(cfg.ReadDuplicateNth))
	atomic.StoreUint32(&ne.writeDuplicateNth, uint32(cfg.WriteDuplicateNth))
	atomic.StoreUint32(&ne.readReorderNth, uint32(cfg.ReadReorderNth))
	atomic.StoreUint32(&ne.writeReorderNth, uint32(cfg.WriteReorderNth))
	atomic.StoreUint32(&ne.readCounter, 0)
	atomic.StoreUint32(&ne.writeCounter, 0)
}

func (ne *Netem) Reset() {
	ne.Update(Config{})
}

func (ne *Netem) Close() error {
	ne.mu.Lock()
	defer ne.mu.Unlock()
	if ne.closed.Get() {
		return ErrNetemClosed
	}
	close(ne.die)
	if err := ne.Conn.Close(); err != nil {
		return err
	}
	ne.closed.Set(true)
	ne.wg.Wait()
	return nil
}

func (ne *Netem) readRoutine() {
	defer func() {
		log.Debugf("Read routine done")
		ne.wg.Done()
	}()
	for {
		var res readResult
		n, err := ne.Conn.Read(ne.rawBuffer)
		if err != nil {
			res.err = err
			ne.readError.Store(err)
			return
		}
		res.data = make([]byte, n)
		copy(res.data, ne.rawBuffer)
		ne.readQueue <- res
	}
}

func (ne *Netem) writeRoutine() {
	defer func() {
		log.Debugf("Write routine done")
		ne.wg.Done()
	}()
	for {
		select {
		case wr := <-ne.writeQueue:
			var res writeResult
			res.n, res.err = ne.internalWrite(wr.data)
			wr.result <- res
		case <-ne.die:
			return
		}
	}
}

func (ne *Netem) internalRead(b []byte) (int, error) {
	// Simulate packet fragmentation on reader side
	fs := int(atomic.LoadUint32(&ne.readFragmentSize))
	if fs > 0 && fs < len(b) {
		b = b[:fs]
	}
	for {
		rc := atomic.AddUint32(&ne.readCounter, 1)
		rl := atomic.LoadUint32(&ne.readLossNth)
		rd := atomic.LoadUint32(&ne.readDuplicateNth)
		rr := atomic.LoadUint32(&ne.readReorderNth)

		shouldLoss := rl > 0 && rc%rl == 0
		shouldDupe := rd > 0 && rc%rd == 0
		shouldReorder := rr > 0 && rc%rr == 0

		logFields := logrus.Fields{
			"op":      "read",
			"counter": rc,
		}

		if shouldLoss || shouldReorder {
			// Drain a packet to emulate packet loss
			if shouldLoss {
				log.WithFields(logFields).Debug("Simulating packet loss")
			} else if shouldReorder {
				log.WithFields(logFields).Debug("Simulating packet reordering")
			}
			n, err := ne.buffer.Read(b)
			if err != nil {
				return 0, err
			}
			// Skip this current packet since its packet is lost
			if shouldLoss {
				continue
			}
			// Rewrite to buffer to emulate packet reordering
			if shouldReorder {
				ne.buffer.Write(b[:n])
			}
		}

		n, err := ne.buffer.Read(b)
		if err != nil {
			return 0, err
		}
		log.WithFields(logFields).Debugf("Read %d bytes", n)

		// Shift the buffer with what we just read to emulate packet duplication
		if shouldDupe {
			log.WithFields(logFields).Debug("Simulating packet duplication")
			p := ne.buffer.Bytes()
			temp := make([]byte, n+len(p))
			copy(temp[:n], b)
			copy(temp[n:], p)
			ne.buffer.Reset()
			ne.buffer.Write(temp)
		}
		return n, nil
	}
}

func (ne *Netem) internalWrite(b []byte) (int, error) {
	n := len(b)
	fs := int(atomic.LoadUint32(&ne.writeFragmentSize))
	if fs <= 0 || fs > n {
		fs = n
	}
	w := 0
	for m := len(b); w < n; m = len(b) {
		if fs > m {
			fs = m
		}

		wc := atomic.AddUint32(&ne.writeCounter, 1)
		wl := atomic.LoadUint32(&ne.writeLossNth)
		wd := atomic.LoadUint32(&ne.writeDuplicateNth)
		wr := atomic.LoadUint32(&ne.writeReorderNth)

		shouldLoss := wl > 0 && wc%wl == 0
		shouldDupe := wd > 0 && wc%wd == 0
		shouldReorder := wr > 0 && wc%wr == 0

		logFields := logrus.Fields{
			"op":      "write",
			"counter": wc,
		}

		if shouldLoss {
			// Increase counter up to fragment size to simulate packet loss
			log.WithFields(logFields).Debug("Simulating packet loss")
			b = b[fs:]
			w += fs
			continue
		} else if shouldReorder && fs < m {
			// Reslice the buffer to simulate packet reordering
			log.WithFields(logFields).Debug("Simulating packet reordering")
			temp := make([]byte, m)
			nn := copy(temp, b[fs:])
			copy(temp[nn:], b)
			b = temp
		}

		nn, err := ne.Conn.Write(b[:fs])
		if err != nil {
			return 0, err
		}
		log.WithFields(logFields).Debugf("Wrote %d bytes", nn)

		// Don't increase the write count to write the same packet again
		// next loop to emulate packet duplication
		if shouldDupe {
			log.WithFields(logFields).Debug("Simulating packet duplication")
			nn = 0
		}

		b = b[nn:]
		w += nn
	}
	return w, nil
}
