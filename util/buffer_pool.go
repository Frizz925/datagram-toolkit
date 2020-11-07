package util

import "sync"

type BufferPool struct {
	bufferSize   int
	preallocSize int

	buffers  [][]byte
	bufferCh chan []byte

	mu sync.Mutex
}

func NewBufferPool(bufferSize int, preallocSize int) *BufferPool {
	if bufferSize <= 0 {
		panic("Buffer size must be greater than zero")
	}
	if preallocSize < 0 {
		preallocSize = 0
	}
	bp := &BufferPool{
		bufferSize:   bufferSize,
		preallocSize: preallocSize,
	}
	if preallocSize > 0 {
		bp.bufferCh = make(chan []byte, preallocSize)
		for i := 0; i < preallocSize; i++ {
			bp.bufferCh <- make([]byte, bufferSize)
		}
	}
	return bp
}

func (bp *BufferPool) Get() []byte {
	if bp.preallocSize > 0 {
		return <-bp.bufferCh
	}
	bp.mu.Lock()
	defer bp.mu.Unlock()
	if len(bp.buffers) > 0 {
		b := bp.buffers[0]
		bp.buffers = bp.buffers[1:]
		return b
	}
	return make([]byte, bp.bufferSize)
}

func (bp *BufferPool) Put(b []byte) {
	if len(b) != bp.bufferSize || cap(b) != bp.bufferSize {
		panic("Trying to put buffer with invalid size into pool")
	}
	if bp.preallocSize > 0 {
		bp.bufferCh <- b
		return
	}
	bp.mu.Lock()
	bp.buffers = append(bp.buffers, b)
	bp.mu.Unlock()
}
