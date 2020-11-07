package mux

import (
	"io"
)

const (
	defaultAcceptBacklog     = 5
	defaultInitialBufferSize = 4096
	defaultReadBufferSize    = 65535
	defaultReadBacklog       = 1024
	defaultWriteBacklog      = 1024

	minBufferSize = 512
)

type Muxer interface {
	Open() (Stream, error)
	Accept() (Stream, error)
	Close() error
}

type Config struct {
	AcceptBacklog     int
	InitialBufferSize int
	ReadBufferSize    int
	ReadBacklog       int
	WriteBacklog      int
}

type writeRequest struct {
	frame
	result chan<- writeResult
}

type writeResult struct {
	n   int
	err error
}

func DefaultConfig() Config {
	return Config{
		AcceptBacklog:  defaultAcceptBacklog,
		ReadBufferSize: defaultReadBufferSize,
		ReadBacklog:    defaultReadBacklog,
		WriteBacklog:   defaultWriteBacklog,
	}
}

func sanitizeConfig(cfg Config) Config {
	if cfg.AcceptBacklog < 0 {
		cfg.AcceptBacklog = 0
	}
	if cfg.InitialBufferSize < minBufferSize {
		cfg.InitialBufferSize = minBufferSize
	}
	if cfg.ReadBufferSize < minBufferSize {
		cfg.InitialBufferSize = minBufferSize
	}
	if cfg.ReadBacklog < 0 {
		cfg.ReadBacklog = 0
	}
	if cfg.WriteBacklog < 0 {
		cfg.WriteBacklog = 0
	}
	return cfg
}

func Mux(rwc io.ReadWriteCloser, cfg Config) Muxer {
	return NewIOMuxer(rwc, cfg)
}
