package udp

import (
	"datagram-toolkit/util/math"
	"sync"
	"time"
)

type RTTEstimator struct {
	sendTime time.Time

	min      time.Duration
	smoothed time.Duration
	variance time.Duration

	mu sync.RWMutex
}

func (re *RTTEstimator) MinRTT() time.Duration {
	re.mu.RLock()
	defer re.mu.RUnlock()
	return re.min
}

func (re *RTTEstimator) RTTVar() time.Duration {
	re.mu.RLock()
	defer re.mu.RUnlock()
	return re.variance
}

func (re *RTTEstimator) SmoothedRTT() time.Duration {
	re.mu.RLock()
	defer re.mu.RUnlock()
	return re.smoothed
}

func (re *RTTEstimator) UpdateSend() {
	re.mu.Lock()
	defer re.mu.Unlock()
	re.sendTime = time.Now()
}

func (re *RTTEstimator) UpdateRecv() {
	re.mu.Lock()
	defer re.mu.Unlock()

	if re.sendTime.IsZero() {
		return
	}
	rtt := time.Since(re.sendTime)

	// Initialize/update the min_rtt
	if re.min <= 0 || re.min > rtt {
		re.min = rtt
	}

	// Initialize/update the smoothed_rtt and rttvar
	if re.smoothed <= 0 {
		re.smoothed = rtt
	} else {
		re.smoothed = (re.smoothed * 7 / 8) + (rtt / 8)
	}
	if re.variance <= 0 {
		re.variance = rtt / 2
	} else {
		sample := time.Duration(math.AbsDuration(re.smoothed - rtt))
		re.variance = (re.variance * 3 / 4) + (sample / 4)
	}
}
