package protocol

import "encoding/binary"

type HandshakeAck [HandshakeAckSize]byte

func NewHandshakeAck(windowSize, frameSize uint32) HandshakeAck {
	var ack HandshakeAck
	binary.BigEndian.PutUint32(ack[:], windowSize)
	binary.BigEndian.PutUint32(ack[4:], frameSize)
	return ack
}

func (ack HandshakeAck) WindowSize() uint32 {
	return binary.BigEndian.Uint32(ack[:])
}

func (ack HandshakeAck) FrameSize() uint32 {
	return binary.BigEndian.Uint32(ack[4:])
}
