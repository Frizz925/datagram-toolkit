package frame

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRaw(t *testing.T) {
	require := require.New(t)
	buf := &bytes.Buffer{}
	expected := []byte("This is the expected content")

	require.Nil(WriteRaw(buf, expected))
	actual, err := ReadRaw(buf)
	require.Nil(err)
	require.Equal(expected, actual)
}
