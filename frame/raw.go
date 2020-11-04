package frame

import (
	uio "datagram-toolkit/util/io"
	"io"
)

func ReadRaw(r io.Reader) ([]byte, error) {
	length, err := ReadVarInt(r)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, length)
	n, err := io.ReadFull(r, buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], err
}

func WriteRaw(w io.Writer, buf []byte) error {
	length := uint(len(buf))
	if err := WriteVarInt(w, length); err != nil {
		return err
	}
	return uio.WriteFull(w, buf)
}
