package math

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAbs(t *testing.T) {
	t.Run("AbsInt64", func(t *testing.T) {
		require := require.New(t)
		require.Equal(int64(1), AbsInt64(-1))
		require.Equal(int64(1), AbsInt64(1))
	})

	t.Run("AbsDuration", func(t *testing.T) {
		require := require.New(t)
		require.Equal(time.Duration(1), AbsDuration(-1))
		require.Equal(time.Duration(1), AbsDuration(1))
	})
}
