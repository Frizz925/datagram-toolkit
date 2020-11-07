package util

import (
	"io"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBufferPool(t *testing.T) {
	rand := rand.New(rand.NewSource(0))
	bufferSize := 16

	t.Run("invalid buffer size", func(t *testing.T) {
		require.Panics(t, func() {
			NewBufferPool(-1, 0)
		})
	})

	t.Run("static pool", func(t *testing.T) {
		require := require.New(t)
		bp := NewBufferPool(bufferSize, 5)
		b := bp.Get()
		require.Len(b, bufferSize)
		_, err := io.ReadFull(rand, b)
		require.Nil(err)
		bp.Put(b)
		// Expect to return new buffer from preallocated pool
		require.NotEqual(b, bp.Get())
	})

	t.Run("dynamic pool", func(t *testing.T) {
		require := require.New(t)
		bp := NewBufferPool(bufferSize, 0)
		b := bp.Get()
		require.Len(b, bufferSize)
		_, err := io.ReadFull(rand, b)
		require.Nil(err)
		bp.Put(b)
		// Expect to return same buffer from dynamic pool
		require.Equal(b, bp.Get())
	})
}
