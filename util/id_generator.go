package util

import "sync/atomic"

type IDGenerator struct {
	v uint32
}

func (idg *IDGenerator) Next() uint32 {
	return atomic.AddUint32(&idg.v, 1)
}
