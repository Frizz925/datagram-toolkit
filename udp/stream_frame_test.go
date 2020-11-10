package udp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStreamFrame(t *testing.T) {
	t.Run("streamHdr", func(t *testing.T) {
		require := require.New(t)
		seq := uint16(1)
		flags := flagACK | flagFIN
		hdr := newStreamHdr(seq, flags, cmdSYN)
		require.Equal(seq, hdr.Seq())
		require.Equal(flags, hdr.Flags())
		require.Equal(cmdSYN, hdr.Cmd())
	})

	t.Run("handshakeAck", func(t *testing.T) {
		require := require.New(t)
		frameSize := uint32(16)
		windowSize := uint32(32)
		hack := newHandshakeAck(windowSize, frameSize)
		require.Equal(frameSize, hack.FrameSize())
		require.Equal(windowSize, hack.WindowSize())
	})

	t.Run("streamDataHdr", func(t *testing.T) {
		require := require.New(t)
		off := uint32(256)
		size := uint32(512)
		sdh := newStreamDataHdr(off, size)
		require.Equal(off, sdh.Off())
		require.Equal(size, sdh.Len())
	})
}
