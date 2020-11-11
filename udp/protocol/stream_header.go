package protocol

import "encoding/binary"

type StreamHdr [StreamHdrSize]byte

func NewStreamHdr(seq uint16, flags, cmd uint8) StreamHdr {
	var hdr StreamHdr
	binary.BigEndian.PutUint16(hdr[:], seq)
	hdr[2] = (flags & 0xF << 4) | (cmd & 0xF)
	return hdr
}

func (hdr StreamHdr) Seq() uint16 {
	return binary.BigEndian.Uint16(hdr[:])
}

func (hdr StreamHdr) Flags() uint8 {
	return hdr[2] >> 4 & 0xF
}

func (hdr StreamHdr) Cmd() uint8 {
	return hdr[2] & 0xF
}
