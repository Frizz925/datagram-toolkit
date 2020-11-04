package mocks

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConn(t *testing.T) {
	require := require.New(t)
	c1, c2 := Conn()
	expected := []byte("Hello, world!")
	buf := make([]byte, 512)

	w, err := c1.Write(expected)
	require.Nil(err)
	require.Equal(len(expected), w)

	r, err := c2.Read(buf)
	require.Nil(err)
	require.Equal(len(expected), r)

	actual := buf[:r]
	require.Equal(expected, actual)

	require.Nil(c1.Close())
	require.Nil(c2.Close())
}
