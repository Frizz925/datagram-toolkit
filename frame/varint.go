package frame

import (
	"bytes"
	utilio "datagram-toolkit/util/io"
	"encoding/binary"
	"errors"
	"io"
	"math"
)

const (
	flagUint6 uint8 = iota
	flagUint14
	flagUint30
	flagUint62
)

const (
	maxUint6  = math.MaxUint8 / 4
	maxUint14 = math.MaxUint16 / 4
	maxUint30 = math.MaxUint32 / 4
	maxUint62 = math.MaxUint64 / 4
)

var ErrVarIntTooLarge = errors.New("value too large to encode into varint")

func WriteVarInt(w io.Writer, v uint) error {
	buf := make([]byte, 8)
	flag := uint8(0)
	switch {
	case v > maxUint62:
		return ErrVarIntTooLarge
	case v > maxUint30:
		binary.BigEndian.PutUint64(buf, uint64(v))
		flag = flagUint62
	case v > maxUint14:
		binary.BigEndian.PutUint32(buf, uint32(v))
		buf = buf[:4]
		flag = flagUint30
	case v > maxUint6:
		binary.BigEndian.PutUint16(buf, uint16(v))
		buf = buf[:2]
		flag = flagUint14
	default:
		buf[0] = byte(v)
		buf = buf[:1]
		flag = flagUint6
	}
	buf[0] = ((flag & 0x03) << 6) | (buf[0] & 0x3F)
	return utilio.WriteFull(w, buf)
}

func ReadVarInt(r io.Reader) (uint, error) {
	firstByte, err := utilio.ReadByte(r)
	if err != nil {
		return 0, err
	}
	buf := &bytes.Buffer{}
	buf.WriteByte(firstByte & 0x3F)
	switch uint8((firstByte >> 6) & 0x03) {
	case flagUint62:
		if _, err := io.CopyN(buf, r, 7); err != nil {
			return 0, err
		}
		return uint(binary.BigEndian.Uint64(buf.Bytes())), nil
	case flagUint30:
		if _, err := io.CopyN(buf, r, 3); err != nil {
			return 0, err
		}
		return uint(binary.BigEndian.Uint32(buf.Bytes())), nil
	case flagUint14:
		if _, err := io.CopyN(buf, r, 1); err != nil {
			return 0, err
		}
		return uint(binary.BigEndian.Uint16(buf.Bytes())), nil
	default:
		return uint(buf.Bytes()[0]), nil
	}
}
