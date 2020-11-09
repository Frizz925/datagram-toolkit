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

	re.UpdateSend(1)
	time.Sleep(delay)

	re.UpdateRecv(2)
	require.Equal(int64(0), re.MinRTT().Nanoseconds())
	require.Equal(int64(0), re.RTTVar().Nanoseconds())
	require.Equal(int64(0), re.SmoothedRTT().Nanoseconds())

	re.UpdateRecv(1)
	require.LessOrEqual(delay.Nanoseconds(), re.MinRTT().Nanoseconds())
	require.LessOrEqual(delay.Nanoseconds(), re.SmoothedRTT().Nanoseconds())
	require.LessOrEqual(delay.Nanoseconds()/2, re.RTTVar().Nanoseconds())

	re.UpdateSend(2)
	time.Sleep(delay)
	re.UpdateRecv(2)
	require.LessOrEqual(delay.Nanoseconds(), re.MinRTT().Nanoseconds())
	require.LessOrEqual(delay.Nanoseconds(), re.SmoothedRTT().Nanoseconds())
	require.Greater(delay.Nanoseconds()/2, re.RTTVar().Nanoseconds())
}
