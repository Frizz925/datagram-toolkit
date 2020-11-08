package udp

func (s *Stream) read(b []byte) (int, error) {
	s.readerLock.Lock()
	defer s.readerLock.Unlock()
	return s.reader.Read(b)
}

func (s *Stream) bufferRead(b []byte) (int, error) {
	s.bufferLock.Lock()
	defer s.bufferLock.Unlock()
	return s.buffer.Read(b)
}

func (s *Stream) bufferLen() int {
	s.bufferLock.Lock()
	defer s.bufferLock.Unlock()
	return s.buffer.Len()
}
