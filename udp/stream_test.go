package udp

import (
	"datagram-toolkit/netem"
	"datagram-toolkit/util/mocks"
	"io"
	"math/rand"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStream(t *testing.T) {
	logger := discardLogger
	if os.Getenv("VERBOSE_TEST") == "1" {
		logger = stderrLogger
	}

	expectedLen := 512
	netemCfg := netem.Config{
		WriteFragmentSize: 64,
		WriteReorderNth:   2,
		WriteDuplicateNth: 3,
		WriteLossNth:      5,
	}
	streamCfg := StreamConfig{
		WindowSize: expectedLen / 2,
		Logger:     logger,
	}

	rand := rand.New(rand.NewSource(0))
	expected := make([]byte, expectedLen)
	buf := make([]byte, expectedLen)

	c1, c2 := mocks.Conn()
	s1 := NewStream(netem.New(c1, netemCfg), streamCfg)
	s2 := NewStream(netem.New(c2, netemCfg), streamCfg)

	require := require.New(t)
	_, err := io.ReadFull(rand, expected)
	require.Nil(err)

	w, err := s1.Write(expected)
	require.Nil(err)
	require.Greater(w, 0)

	r, err := s2.Read(buf)
	require.Nil(err, "Read error")
	require.Equal(w, r, "Read/write count mismatch")
	require.Equal(expected[:r], buf[:r], "Content mismatch")
	require.Nil(s2.Reset(), "Reset error")

	s1.Close()
	s2.Close()
}
