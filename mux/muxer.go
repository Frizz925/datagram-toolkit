package mux

import "io"

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

type readResult struct {
	data []byte
	head []byte
	err  error
}

func Mux(rwc io.ReadWriteCloser, cfg Config) Muxer {
	return NewIOMuxer(rwc, cfg)
}
