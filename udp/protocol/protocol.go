package protocol

const (
	// u16 Sequence + u8 Flags+Cmd
	StreamHdrSize = 3
	// u32 Window Size + 12 bytes reserved
	HandshakeHdrSize = 16
	// u32 Frame Size + u32 Window Size
	HandshakeAckSize = 8
	// u32 Offset + u32 Length
	DataHdrSize = 8
)

const (
	FlagACK uint8 = 1 << iota
	FlagRST
	FlagFIN
)

const (
	// Command for initiating handshake
	CmdSYN uint8 = iota + 1
	// Command for acknowledging frames
	CmdACK
	// Command for closing stream
	CmdFIN
	// Command for pushing data to peer
	CmdPSH
)
