package protocol

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDataHeader(t *testing.T) {
	require := require.New(t)
	off := uint32(256)
	size := uint32(512)
	dhdr := NewDataHdr(off, size)
	require.Equal(off, dhdr.Off())
	require.Equal(size, dhdr.Len())
}
