package errors

import (
	"errors"
)

var ErrTimeout = errors.New("timeout")

func IsDeadlineError(err error) bool {
	if err == ErrTimeout {
		return true
	}
	err = errors.Unwrap(err)
	if err == nil {
		return false
	}
	return err.Error() == "i/o timeout"
}
