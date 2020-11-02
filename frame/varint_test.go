package frame

import (
	"bytes"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVarInt(t *testing.T) {
	require := require.New(t)
	buf := &bytes.Buffer{}

	// Should be encoded into 1-byte varint
	require.Nil(WriteVarInt(buf, 8))
	require.Equal(1, buf.Len())
	v0, err := ReadVarInt(buf)
	require.Nil(err)
	require.Equal(8, int(v0))
	buf.Reset()

	// Should be encoded into 2-bytes varint
	require.Nil(WriteVarInt(buf, uint(math.MaxUint8)))
	require.Equal(2, buf.Len())
	v1, err := ReadVarInt(buf)
	require.Nil(err)
	require.Equal(math.MaxUint8, int(v1))
	buf.Reset()

	// Should be encoded into 4-bytes varint
	require.Nil(WriteVarInt(buf, uint(math.MaxUint16)))
	require.Equal(4, buf.Len())
	v2, err := ReadVarInt(buf)
	require.Nil(err)
	require.Equal(math.MaxUint16, int(v2))
	buf.Reset()

	// Should be encoded into 8-bytes varint
	require.Nil(WriteVarInt(buf, uint(math.MaxUint32)))
	require.Equal(8, buf.Len())
	v3, err := ReadVarInt(buf)
	require.Nil(err)
	require.Equal(math.MaxUint32, int(v3))
	buf.Reset()

	// Error when encoding larger than uint62 varint
	require.Equal(ErrVarIntTooLarge, WriteVarInt(buf, uint(math.MaxUint64)))
}
