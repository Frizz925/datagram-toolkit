package udp

import (
	"datagram-toolkit/util/math"
	"sync"
	"time"
)

type RTTStats struct {
	sendTime time.Time

	min      time.Duration
	smoothed time.Duration
	variance time.Duration

	mu sync.RWMutex
}

func (rs *RTTStats) Min() time.Duration {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.min
}

func (rs *RTTStats) Var() time.Duration {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.variance
}

func (rs *RTTStats) Smoothed() time.Duration {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.smoothed
}

func (rs *RTTStats) UpdateSend() {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.sendTime = time.Now()
}

func (rs *RTTStats) UpdateRecv() {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.sendTime.IsZero() {
		return
	}
	rtt := time.Since(rs.sendTime)

	// Initialize/update the min_rtt
	if rs.min <= 0 || rs.min > rtt {
		rs.min = rtt
	}

	// Initialize/update the smoothed_rtt and rttvar
	if rs.smoothed <= 0 {
		rs.smoothed = rtt
	} else {
		rs.smoothed = (rs.smoothed * 7 / 8) + (rtt / 8)
	}
	if rs.variance <= 0 {
		rs.variance = rtt / 2
	} else {
		sample := time.Duration(math.AbsDuration(rs.smoothed - rtt))
		rs.variance = (rs.variance * 3 / 4) + (sample / 4)
	}
}
