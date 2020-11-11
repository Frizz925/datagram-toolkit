package protocol

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandshakeHeader(t *testing.T) {
	wsize := uint32(2048)
	hdr := NewHandshakeHdr(wsize)
	require.Equal(t, wsize, hdr.WindowSize())
}
