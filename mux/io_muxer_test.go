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
	chRwc := make(chan io.ReadWriteCloser, 2)
	chBuf := make(chan []byte, 2)
	chErr := make(chan error, 2)
	go muxAccept(ms, chRwc, chErr)

	t.Cleanup(func() {
		require := require.New(t)
		require.Nil(ms.Close())
		require.Nil(mc.Close())
	})

	var err error
	t.Run("setup", func(t *testing.T) {
		rand := rand.New(rand.NewSource(0))
		_, err = io.ReadFull(rand, expected1)
		require.Nil(t, err)
		_, err = io.ReadFull(rand, expected2)
		require.Nil(t, err)
	})

	var ss1, ss2, sc1, sc2 io.ReadWriteCloser
	t.Run("open and accept stream", func(t *testing.T) {
		require := require.New(t)

		// Client streams
		sc1, err = mc.Open()
		require.Nil(err)
		sc2, err = mc.Open()
		require.Nil(err)

		// Server streams
		ss1, err = <-chRwc, <-chErr
		require.Nil(err)
		ss2, err = <-chRwc, <-chErr
		require.Nil(err)
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

		// Close client-side
		require.Nil(sc1.Close())
		require.Nil(sc2.Close())
		require.Nil(mc.Close())

		// Close server-side
		require.Equal(io.ErrClosedPipe, ss1.Close())
		require.Equal(io.ErrClosedPipe, ss2.Close())
		require.Nil(ms.Close())
	})
}

func muxAccept(m Muxer, chRwc chan<- io.ReadWriteCloser, chErr chan<- error) {
	for {
		rwc, err := m.Accept()
		chRwc <- rwc
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
