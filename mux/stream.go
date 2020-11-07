package mux

import (
	"io"
)

type Stream interface {
	io.ReadWriteCloser
	ID() uint32
}
