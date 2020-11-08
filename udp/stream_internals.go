package udp

import (
	"datagram-toolkit/util"
	"io"
)

func (s *Stream) internalRead() error {
	var hdr streamHdr
	_, err := s.read(hdr[:])
	if err != nil {
		return nil
	}
	isAck := hdr.Flags()&flagACK != 0
	switch hdr.Cmd() {
	case cmdSYN:
		return s.handleHandshake(isAck)
	case cmdFIN:
		//nolint:errcheck
		go s.internalClose(false)
	case cmdRST:
		s.handleReset(isAck)
	case cmdPSH:
		return s.handleData(hdr.Flags())
	}
	return nil
}

func (s *Stream) internalWrite(b []byte, flush bool) (n int, err error) {
	s.writerLock.Lock()
	defer s.writerLock.Unlock()
	n, err = s.writer.Write(b)
	if err != nil {
		return 0, err
	}
	if flush {
		err = s.writer.Flush()
	}
	return n, err
}

func (s *Stream) internalReset() {
	// TODO: Maybe use different error for stream reset
	util.AsyncNotifyErr(s.readErrCh, io.EOF)
	util.AsyncNotifyErr(s.writeErrCh, io.EOF)

	s.readerLock.Lock()
	s.reader.Reset(s.Conn)
	s.readerLock.Unlock()

	s.writerLock.Lock()
	s.writer.Reset(s.Conn)
	s.writerLock.Unlock()

	s.bufferLock.Lock()
	s.buffer.Reset()
	s.bufferLock.Unlock()
}

func (s *Stream) internalClose(sendFin bool) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sendFin {
		if _, err = s.writeFrame(0, cmdFIN); err != nil {
			return err
		}
	}
	s.closeOnce.Do(func() {
		close(s.die)
	})
	s.wg.Wait()
	return s.Conn.Close()
}
