package udp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRTTEstimator(t *testing.T) {
	var re RTTEstimator
	require := require.New(t)
	delay := 125 * time.Millisecond

	re.UpdateSend()
	require.Equal(int64(0), re.MinRTT().Nanoseconds())
	require.Equal(int64(0), re.RTTVar().Nanoseconds())
	require.Equal(int64(0), re.SmoothedRTT().Nanoseconds())

	time.Sleep(delay)
	re.UpdateRecv()
	require.LessOrEqual(delay.Nanoseconds(), re.MinRTT().Nanoseconds())
	require.LessOrEqual(delay.Nanoseconds(), re.SmoothedRTT().Nanoseconds())
	require.LessOrEqual(delay.Nanoseconds()/2, re.RTTVar().Nanoseconds())

	re.UpdateSend()
	time.Sleep(delay)
	re.UpdateRecv()
	require.LessOrEqual(delay.Nanoseconds(), re.MinRTT().Nanoseconds())
	require.LessOrEqual(delay.Nanoseconds(), re.SmoothedRTT().Nanoseconds())
	require.Greater(delay.Nanoseconds()/2, re.RTTVar().Nanoseconds())
}
