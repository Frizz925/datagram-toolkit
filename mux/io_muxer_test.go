package mux

import (
	"datagram-toolkit/util/mocks"
	"io"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIOMuxer(t *testing.T) {
	expectedLen := 64
	expected1 := make([]byte, expectedLen)
	expected2 := make([]byte, expectedLen)
	cfg := DefaultConfig()

	s, c := mocks.Conn()
	ms, mc := NewIOMuxer(s, cfg), NewIOMuxer(c, cfg)
	chStr := make(chan Stream, 2)
	chBuf := make(chan []byte, 2)
	chErr := make(chan error, 2)
	go muxAccept(ms, chStr, chErr)

	t.Cleanup(func() {
		ms.Close()
		mc.Close()
	})

	var err error
	t.Run("setup", func(t *testing.T) {
		rand := rand.New(rand.NewSource(0))
		_, err = io.ReadFull(rand, expected1)
		require.Nil(t, err)
		_, err = io.ReadFull(rand, expected2)
		require.Nil(t, err)
	})

	var ss1, ss2, sc1, sc2 Stream
	t.Run("open and accept stream", func(t *testing.T) {
		require := require.New(t)

		// First pair of streams
		sc1, err = mc.Open()
		require.Nil(err)
		ss1, err = <-chStr, <-chErr
		require.Nil(err)

		// Second pair of streams
		sc2, err = mc.Open()
		require.Nil(err)
		ss2, err = <-chStr, <-chErr
		require.Nil(err)

		// Read routines
		go streamRead(ss1, chBuf, chErr)
		go streamRead(ss2, chBuf, chErr)
	})

	var n int
	t.Run("send to and receive from multiple streams", func(t *testing.T) {
		require := require.New(t)

		n, err = sc1.Write(expected1)
		require.Nil(err)
		require.Equal(expectedLen, n)

		n, err = sc2.Write(expected2)
		require.Nil(err)
		require.Equal(expectedLen, n)

		require.Nil(<-chErr)
		require.Nil(<-chErr)
		require.Equal(expected1, <-chBuf)
		require.Equal(expected2, <-chBuf)
	})

	t.Run("cleanup", func(t *testing.T) {
		require := require.New(t)

		require.Nil(sc1.Close())
		require.Equal(io.ErrClosedPipe, ss1.Close())
		require.Nil(ss2.Close())
		require.Equal(io.ErrClosedPipe, sc2.Close())

		require.Nil(mc.Close())
		require.Nil(ms.Close())
	})
}

func muxAccept(m Muxer, chStr chan<- Stream, chErr chan<- error) {
	for {
		str, err := m.Accept()
		chStr <- str
		chErr <- err
		if err != nil {
			return
		}
	}
}

func streamRead(s Stream, chBuf chan<- []byte, chErr chan<- error) {
	buf := make([]byte, 512)
	for {
		n, err := s.Read(buf)
		b := make([]byte, n)
		copy(b, buf)
		chBuf <- b
		chErr <- err
		if err != nil {
			return
		}
	}
}
