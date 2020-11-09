package math

import "time"

func AbsInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func AbsDuration(v time.Duration) time.Duration {
	if v < 0 {
		return -v
	}
	return v
}
