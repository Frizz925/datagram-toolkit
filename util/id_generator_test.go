package util

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIDGenerator(t *testing.T) {
	var idg IDGenerator

	t.Run("sync", func(t *testing.T) {
		require := require.New(t)
		require.EqualValues(1, idg.Next())
		require.EqualValues(2, idg.Next())
		require.EqualValues(3, idg.Next())
	})

	t.Run("async", func(t *testing.T) {
		wg := &sync.WaitGroup{}
		for i := 0; i < 3; i++ {
			wg.Add(1)
			go asyncNextGenerator(wg, &idg)
		}
		wg.Wait()
		require.EqualValues(t, 10, idg.Next())
	})
}

func asyncNextGenerator(wg *sync.WaitGroup, idg *IDGenerator) {
	defer wg.Done()
	for j := 0; j < 2; j++ {
		idg.Next()
	}
}
