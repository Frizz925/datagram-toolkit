package udp

import "encoding/binary"

const (
	// u16 Sequence + u8 Flags+Cmd
	szStreamHdr = 3
	// u32 Frame Size + u32 Window Size
	szHandshakeAck = 8
	// u16 Sequence + u32 Offset + u32 Length
	szStreamDataHdr = 10
	// u16 Sequence + u32 Consumed
	szStreamDataAck = 6
)

const (
	flagACK uint8 = 1 << iota
	flagFIN
)

const (
	// Command for initiating handshake
	cmdSYN uint8 = iota + 1
	// Command for closing stream
	cmdFIN
	// Command for resetting stream
	cmdRST
	// Command for pushing data to peer
	cmdPSH
)

type streamHdr [szStreamHdr]byte

func newStreamHdr(seq uint16, flags, cmd uint8) streamHdr {
	var hdr streamHdr
	binary.BigEndian.PutUint16(hdr[:], seq)
	hdr[2] = (flags & 0xF << 4) | (cmd & 0xF)
	return hdr
}

func (hdr streamHdr) Seq() uint16 {
	return binary.BigEndian.Uint16(hdr[:])
}

func (hdr streamHdr) Flags() uint8 {
	return hdr[2] >> 4 & 0xF
}

func (hdr streamHdr) Cmd() uint8 {
	return hdr[2] & 0xF
}

type handshakeAck [szHandshakeAck]byte

func newHandshakeAck(windowSize, frameSize uint32) handshakeAck {
	var hack handshakeAck
	binary.BigEndian.PutUint32(hack[:], windowSize)
	binary.BigEndian.PutUint32(hack[4:], frameSize)
	return hack
}

func (hack handshakeAck) WindowSize() uint32 {
	return binary.BigEndian.Uint32(hack[:])
}

func (hack handshakeAck) FrameSize() uint32 {
	return binary.BigEndian.Uint32(hack[4:])
}

type streamDataHdr [szStreamDataHdr]byte

func newStreamDataHdr(seq uint16, offset, length uint32) streamDataHdr {
	var sdh streamDataHdr
	binary.BigEndian.PutUint16(sdh[:], seq)
	binary.BigEndian.PutUint32(sdh[2:], offset)
	binary.BigEndian.PutUint32(sdh[6:], length)
	return sdh
}

func (sdh streamDataHdr) Seq() uint16 {
	return binary.BigEndian.Uint16(sdh[:])
}

func (sdh streamDataHdr) Off() uint32 {
	return binary.BigEndian.Uint32(sdh[2:])
}

func (sdh streamDataHdr) Len() uint32 {
	return binary.BigEndian.Uint32(sdh[6:])
}

type streamDataAck [szStreamDataAck]byte

func newStreamDataAck(seq uint16, consumed uint32) streamDataAck {
	var sda streamDataAck
	binary.BigEndian.PutUint16(sda[:], seq)
	binary.BigEndian.PutUint32(sda[2:], consumed)
	return sda
}

func (sda streamDataAck) Seq() uint16 {
	return binary.BigEndian.Uint16(sda[:])
}

func (sda streamDataAck) Consumed() uint32 {
	return binary.BigEndian.Uint32(sda[2:])
}
