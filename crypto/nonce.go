package crypto

import (
	uio "datagram-toolkit/util/io"
	"encoding/binary"
	"io"
	"sync/atomic"
)

type NonceReadWriter interface {
	WriteNonce(w io.Writer) error
	ReadNonce(r io.Reader) ([]byte, error)
}

type sequentialNonceReadWriter struct {
	size int
	seq  uint64
}

func (snrw *sequentialNonceReadWriter) ReadNonce(r io.Reader) ([]byte, error) {
	nonce := make([]byte, snrw.size)
	_, err := io.ReadFull(r, nonce)
	return nonce, err
}

func (snrw *sequentialNonceReadWriter) WriteNonce(w io.Writer) error {
	nonce := make([]byte, snrw.size)
	seq := atomic.AddUint64(&snrw.seq, 1)
	if snrw.size < 2 {
		nonce[0] = byte(seq)
	} else if snrw.size < 4 {
		binary.LittleEndian.PutUint16(nonce, uint16(seq))
	} else if snrw.size < 8 {
		binary.LittleEndian.PutUint32(nonce, uint32(seq))
	} else {
		binary.LittleEndian.PutUint64(nonce, seq)
	}
	return uio.WriteFull(w, nonce)
}
