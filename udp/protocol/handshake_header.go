package protocol

import "encoding/binary"

type HandshakeHdr [HandshakeHdrSize]byte

func NewHandshakeHdr(wsize uint32) HandshakeHdr {
	var hdr HandshakeHdr
	binary.BigEndian.PutUint32(hdr[:], wsize)
	return hdr
}

func (hdr HandshakeHdr) WindowSize() uint32 {
	return binary.BigEndian.Uint32(hdr[:])
}
