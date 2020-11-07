package mux

import "encoding/binary"

// uint32 StreamID + uint8 CMD
const headerSize = 5

const (
	// SYN command for opening/accepting new stream
	cmdSYN uint8 = iota + 1
	// STD command for sending data in a stream
	cmdSTD
	// FIN command for closing stream
	cmdFIN
)

type frame []byte

func newFrame(id uint32, cmd uint8, body []byte) frame {
	buf := make([]byte, headerSize+len(body))
	binary.BigEndian.PutUint32(buf[:4], id)
	buf[4] = cmd
	copy(buf[5:], body)
	return frame(buf)
}

func (f frame) StreamID() uint32 {
	return binary.BigEndian.Uint32(f)
}

func (f frame) Cmd() uint8 {
	return f[4]
}

func (f frame) Body() []byte {
	return f[headerSize:]
}

func (f frame) Len() int {
	return len(f) - headerSize
}
