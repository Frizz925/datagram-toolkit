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
	logger := stderrLogger
	expectedLen := 2048
	netemCfg := netem.Config{
		WriteFragmentSize: 48,
		WriteDuplicateNth: 3,
		WriteReorderNth:   5,
		WriteLossNth:      7,
	}
	streamCfg := StreamConfig{
		WindowSize: expectedLen / 2,
	}
	streamCfg.Logger = logger

	rand := rand.New(rand.NewSource(0))
	expected := make([]byte, expectedLen)
	buf := make([]byte, expectedLen)

	c1, c2 := mocks.Conn()
	s1 := NewStream(netem.New(c1, netemCfg), streamCfg)
	s2 := NewStream(netem.New(c2, netemCfg), streamCfg)

	require := require.New(t)
	_, err := io.ReadFull(rand, expected)
	require.Nil(err)

	for i := 1; i <= 2; i++ {
		w, err := s1.Write(expected)
		require.Nil(err)
		require.Greater(w, 0)

		n, err := s1.Write(expected[w:])
		require.Nil(err)
		require.Greater(n, 0)

		r, err := s2.Read(buf)
		require.Nil(err, "Read error at run %d", i)
		require.Equal(w, r, "Read/write count mismatch at run %d", i)
		require.Equal(expected[:r], buf[:r], "Content mismatch at run %d", i)
		require.Nil(s2.Reset(), "Reset error at run %d", i)
	}

	s1.Close()
	s2.Close()
}
