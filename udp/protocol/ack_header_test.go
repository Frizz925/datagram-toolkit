package protocol

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAckHeader(t *testing.T) {
	length := uint16(16)
	hdr := NewAckHdr(length)
	require.Equal(t, length, hdr.Len())
}
