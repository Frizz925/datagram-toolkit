package util

func AsyncNotify(ch chan<- struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func AsyncNotifyErr(ch chan<- error, err error) {
	select {
	case ch <- err:
	default:
	}
}
