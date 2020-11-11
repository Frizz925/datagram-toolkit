package protocol

import "encoding/binary"

type DataHdr [DataHdrSize]byte

func NewDataHdr(offset, length uint32) DataHdr {
	var hdr DataHdr
	binary.BigEndian.PutUint32(hdr[:], offset)
	binary.BigEndian.PutUint32(hdr[4:], length)
	return hdr
}

func (hdr DataHdr) Off() uint32 {
	return binary.BigEndian.Uint32(hdr[:])
}

func (hdr DataHdr) Len() uint32 {
	return binary.BigEndian.Uint32(hdr[4:])
}
