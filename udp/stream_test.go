package udp

import (
	"datagram-toolkit/netem"
	"datagram-toolkit/util/mocks"
	"io"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStream(t *testing.T) {
	rand := rand.New(rand.NewSource(0))
	streamCfg := DefaultStreamConfig()
	netemCfg := netem.Config{
		WriteFragmentSize: 48,
		WriteReorderNth:   2,
		WriteDuplicateNth: 3,
		// WriteLossNth:      4,
	}
	expectedLen := 64
	expected := make([]byte, expectedLen)

	c1, c2 := mocks.Conn()
	s1 := NewStream(netem.New(c1, netemCfg), streamCfg)
	s2 := NewStream(c2, streamCfg)
	defer s1.Close()
	defer s2.Close()

	require := require.New(t)
	_, err := io.ReadFull(rand, expected)
	require.Nil(err)

	w, err := s1.Write(expected)
	require.Nil(err)
	require.Equal(expectedLen, w)

	buf := make([]byte, expectedLen*2)
	r, err := s2.Read(buf)
	require.Nil(err)
	require.Equal(expectedLen, r)
	require.Equal(expected, buf[:r])
}
