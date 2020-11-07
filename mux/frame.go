package mux

import "encoding/binary"

// u32 StreamID + u8 CMD
const headerSize = 5

const (
	//nolint:deadcode,unused,varcheck
	cmdNOP uint8 = iota
	// SYN command for opening/accepting new stream
	cmdSYN
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
