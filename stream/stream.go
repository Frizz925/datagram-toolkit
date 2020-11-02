package stream

import (
	"bufio"
	"errors"
	"io"
	"sync"
)

const (
	defaultFrameSize  = 1024
	defaultBufferSize = 4096
	minBufferSize     = 512
)

type Stream struct {
	reader    *bufio.Reader
	writer    *bufio.Writer
	frameSize uint32
	mu        sync.Mutex
}

type StreamConfig struct {
	FrameSize  int
	BufferSize int
}

func DefaultConfig() StreamConfig {
	return StreamConfig{
		FrameSize:  defaultFrameSize,
		BufferSize: defaultBufferSize,
	}
}

func New(rw io.ReadWriter, cfg StreamConfig) *Stream {
	if cfg.BufferSize < minBufferSize {
		cfg.BufferSize = minBufferSize
	}
	return &Stream{
		reader:    bufio.NewReader(rw),
		writer:    bufio.NewWriter(rw),
		frameSize: uint32(cfg.FrameSize),
	}
}

func (s *Stream) Read(b []byte) (int, error) {
	return 0, errors.New("read not implemented")
}

func (s *Stream) Write(b []byte) (int, error) {
	return 0, errors.New("write not implemented")
}
