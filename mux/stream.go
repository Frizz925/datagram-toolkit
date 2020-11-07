package mux

import (
	"errors"
	"io"
)

var ErrStreamExhausted = errors.New("stream exhausted")

type Stream interface {
	io.ReadWriteCloser
}
