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
)

const (
	minBufferSize = 512
	minBacklog    = 1
)

var ErrNetemClosed = errors.New("netem closed")

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

	// The size of emulated packet fragmentations on write.
	// Zero value means no emulation of packet fragmentations.
	WriteFragmentSize int
	// Packet at every nth would be discarded on write to emulate packet loss.
	// Zero value means no emulation of packet loss.
	WriteLossNth int
	// Packet at every nth would be duplicated on write to emulate packet duplication.
	// Zero value means no emulation of packet duplication.
	WriteDuplicateNth int
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

	writeFragmentSize uint32
	writeLossNth      uint32
	writeDuplicateNth uint32

	rawBuffer []byte
	buffer    *bytes.Buffer

	readQueue    chan []byte
	readNotify   chan struct{}
	readCounter  uint32
	readDeadline atomic.Value
	readLock     sync.Mutex

	writeQueue    chan writeRequest
	writeNotify   chan struct{}
	writeCounter  uint32
	writeDeadline atomic.Value
	writeLock     sync.Mutex

	err chan error
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

		readQueue:  make(chan []byte, cfg.Backlog),
		readNotify: make(chan struct{}),

		writeQueue:  make(chan writeRequest, cfg.Backlog),
		writeNotify: make(chan struct{}),

		err: make(chan error, 1),
		die: make(chan struct{}),
	}
	ne.Update(cfg)
	ne.wg.Add(2)
	go ne.readRoutine()
	go ne.writeRoutine()
	return ne
}

func (ne *Netem) Read(b []byte) (int, error) {
	ne.readLock.Lock()
	defer ne.readLock.Unlock()
	if ne.closed.Get() {
		return 0, ErrNetemClosed
	}
	if len(b) <= 0 {
		return 0, nil
	}
	// Simulate packet fragmentation on reader side
	fs := int(atomic.LoadUint32(&ne.readFragmentSize))
	if fs > 0 && fs < len(b) {
		b = b[:fs]
	}
	var deadline <-chan time.Time
	for {
		if ne.buffer.Len() > 0 {
			n, err := ne.internalRead(b)
			if err != nil {
				return 0, err
			} else if n > 0 {
				return n, nil
			}
		}
		if t, ok := ne.readDeadline.Load().(time.Time); ok && !t.IsZero() {
			timer := time.NewTimer(time.Until(t))
			defer timer.Stop()
			deadline = timer.C
		}
		select {
		case b := <-ne.readQueue:
			ne.buffer.Write(b)
		case <-ne.readNotify:
		case <-deadline:
			return 0, uerrors.ErrTimeout
		case <-ne.die:
			return 0, io.EOF
		}
	}
}

func (ne *Netem) Write(b []byte) (int, error) {
	ne.writeLock.Lock()
	defer ne.writeLock.Unlock()
	if ne.closed.Get() {
		return 0, ErrNetemClosed
	}
	if len(b) <= 0 {
		return 0, nil
	}
	ch := make(chan writeResult)
	ne.writeQueue <- writeRequest{b, ch}
	var deadline <-chan time.Time
	for {
		if t, ok := ne.writeDeadline.Load().(time.Time); ok && !t.IsZero() {
			timer := time.NewTimer(time.Until(t))
			defer timer.Stop()
			deadline = timer.C
		}
		select {
		case wr := <-ch:
			return wr.n, wr.err
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
	atomic.StoreUint32(&ne.readLossNth, uint32(cfg.ReadLossNth))
	atomic.StoreUint32(&ne.writeLossNth, uint32(cfg.WriteLossNth))
	atomic.StoreUint32(&ne.readFragmentSize, uint32(cfg.ReadFragmentSize))
	atomic.StoreUint32(&ne.writeFragmentSize, uint32(cfg.WriteFragmentSize))
	atomic.StoreUint32(&ne.readDuplicateNth, uint32(cfg.ReadDuplicateNth))
	atomic.StoreUint32(&ne.writeDuplicateNth, uint32(cfg.WriteDuplicateNth))
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
	select {
	case err := <-ne.err:
		if ner, ok := err.(*net.OpError); ok && ner.Op == "read" {
			return nil
		}
		return err
	default:
		return nil
	}
}

func (ne *Netem) internalRead(b []byte) (int, error) {
	// Simulate packet loss on reader side
	rc := atomic.AddUint32(&ne.readCounter, 1)
	rl := atomic.LoadUint32(&ne.readLossNth)
	shouldLoss := rl > 0 && rc%rl == 0
	if shouldLoss {
		// Drain from buffer to simulate packet loss
		_, err := ne.buffer.Read(b)
		if err != nil {
			return 0, err
		}
		if ne.buffer.Len() > 0 {
			// Re-read there is still any buffer left
			return ne.internalRead(b)
		} else {
			// Else, read nothing
			return 0, nil
		}
	}
	n, err := ne.buffer.Read(b)
	if err != nil {
		return 0, err
	}
	// Simulate packet duplication on reader side
	// Packet duplication and packet loss are mutually exclusive
	rd := atomic.LoadUint32(&ne.readDuplicateNth)
	shouldDupe := rd > 0 && rc%rd == 0
	if shouldDupe {
		// Put what we read into the queue to simulate packet duplication
		d := make([]byte, n)
		copy(d, b[:n])
		ne.readQueue <- d
	}
	return n, nil
}

func (ne *Netem) internalWrite(b []byte) (int, error) {
	// Simulate packet fragmentation on writer side
	n := len(b)
	fs := int(atomic.LoadUint32(&ne.writeFragmentSize))
	// If fragment size is zero or larger than the size of data,
	// then set the fragment size to the size of data
	if fs <= 0 || n < fs {
		fs = n
	}
	var nn, w int
	var err error
	for w < n && len(b) > 0 {
		// Simulate packet loss on writer side
		wc := atomic.AddUint32(&ne.writeCounter, 1)
		wl := atomic.LoadUint32(&ne.writeLossNth)
		shouldLoss := wl > 0 && wc%wl == 0
		if shouldLoss {
			// Skip write to wire on packet loss
			nb := len(b)
			if fs > nb {
				nn = nb
			} else {
				nn = fs
			}
		} else {
			nn, err = ne.Conn.Write(b[:fs])
			if err != nil {
				return 0, err
			}
			// Simulate packet duplication on writer side
			// Packet duplication and packet loss are mutually exclusive
			wd := atomic.LoadUint32(&ne.writeDuplicateNth)
			shouldDupe := wd > 0 && wc%wd == 0
			if shouldDupe {
				// Put what we wrote into the queue to simulate packet duplication
				d := make([]byte, fs)
				copy(d, b[:fs])
				ne.writeQueue <- writeRequest{data: d}
			}
		}
		b = b[nn:]
		w += nn
	}
	return w, nil
}

func (ne *Netem) readRoutine() {
	defer ne.wg.Done()
	for {
		n, err := ne.Conn.Read(ne.rawBuffer)
		if err != nil {
			ne.err <- err
			return
		}
		b := make([]byte, n)
		copy(b, ne.rawBuffer[:n])
		ne.readQueue <- b
	}
}

func (ne *Netem) writeRoutine() {
	defer ne.wg.Done()
	for {
		select {
		case wr := <-ne.writeQueue:
			var res writeResult
			res.n, res.err = ne.internalWrite(wr.data)
			if wr.result != nil {
				wr.result <- res
			}
		case <-ne.die:
			return
		}
	}
}
