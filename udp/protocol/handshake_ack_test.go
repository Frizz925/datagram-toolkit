package protocol

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandshakeAck(t *testing.T) {
	require := require.New(t)
	windowSize := uint32(32)
	frameSize := uint32(16)
	hack := NewHandshakeAck(windowSize, frameSize)
	require.Equal(windowSize, hack.WindowSize())
	require.Equal(frameSize, hack.FrameSize())
}
