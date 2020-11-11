package udp

import "fmt"

func (s *Stream) log(format string, v ...interface{}) {
	prefix := fmt.Sprintf("[%p] ", s)
	message := fmt.Sprintf(format, v...)
	s.logger.Print(prefix + message)
}
