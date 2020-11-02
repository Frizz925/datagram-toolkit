package atomic

import "sync/atomic"

type Bool struct {
	v uint32
}

func (b *Bool) Get() bool {
	return atomic.LoadUint32(&b.v) == 1
}

func (b *Bool) Set(v bool) {
	if v {
		atomic.StoreUint32(&b.v, 1)
	} else {
		atomic.StoreUint32(&b.v, 0)
	}
}
