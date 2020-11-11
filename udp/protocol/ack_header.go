package protocol

import "encoding/binary"

type AckHdr [AckHdrSize]byte

func NewAckHdr(length uint16) AckHdr {
	var hdr AckHdr
	binary.BigEndian.PutUint16(hdr[:], length)
	return hdr
}

func (hdr AckHdr) Len() uint16 {
	return binary.BigEndian.Uint16(hdr[:])
}
