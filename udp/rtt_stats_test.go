package udp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRTTEstimator(t *testing.T) {
	var rs RTTStats
	require := require.New(t)
	delay := 125 * time.Millisecond

	rs.UpdateSend()
	require.Equal(int64(0), rs.Min().Nanoseconds())
	require.Equal(int64(0), rs.Var().Nanoseconds())
	require.Equal(int64(0), rs.Smoothed().Nanoseconds())

	time.Sleep(delay)
	rs.UpdateRecv()
	require.LessOrEqual(delay.Nanoseconds(), rs.Min().Nanoseconds())
	require.LessOrEqual(delay.Nanoseconds(), rs.Smoothed().Nanoseconds())
	require.LessOrEqual(delay.Nanoseconds()/2, rs.Var().Nanoseconds())

	rs.UpdateSend()
	time.Sleep(delay)
	rs.UpdateRecv()
	require.LessOrEqual(delay.Nanoseconds(), rs.Min().Nanoseconds())
	require.LessOrEqual(delay.Nanoseconds(), rs.Smoothed().Nanoseconds())
	require.Greater(delay.Nanoseconds()/2, rs.Var().Nanoseconds())
}
