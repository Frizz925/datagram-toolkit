package util

func AsyncNotify(ch chan<- struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}
