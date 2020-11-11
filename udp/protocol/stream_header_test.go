package protocol

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStreamHeader(t *testing.T) {
	require := require.New(t)
	seq := uint16(1)
	flags := FlagACK | FlagFIN
	cmd := CmdSYN
	hdr := NewStreamHdr(seq, flags, cmd)
	require.Equal(seq, hdr.Seq())
	require.Equal(flags, hdr.Flags())
	require.Equal(cmd, hdr.Cmd())
}
