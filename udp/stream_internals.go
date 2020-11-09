package udp

import (
	"datagram-toolkit/util"
	"sync/atomic"
)

func (s *Stream) internalRead() (err error) {
	s.readerLock.Lock()
	defer s.readerLock.Unlock()
	var hdr streamHdr
	if _, err := s.reader.Read(hdr[:]); err != nil {
		return err
	}
	isAck := hdr.Flags()&flagACK != 0
	switch hdr.Cmd() {
	case cmdSYN:
		err = s.handleHandshake(isAck)
	case cmdFIN:
		//nolint:errcheck
		go s.internalClose(false)
	case cmdRST:
		s.handleReset(isAck)
	case cmdPSH:
		err = s.handleData(hdr.Flags())
	}
	return err
}

func (s *Stream) internalWrite(req streamWriteRequest) (n int, err error) {
	s.writerLock.Lock()
	defer s.writerLock.Unlock()
	hdr := newStreamHdr(req.flags, req.cmd)
	if _, err := s.writer.Write(hdr[:]); err != nil {
		return 0, err
	}
	if len(req.data) > 0 && len(req.head) > 0 {
		n, err = s.writer.Write(req.data)
		if err != nil {
			return 0, err
		}
		s.buffers.Put(req.head)
	}
	if err := s.writer.Flush(); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *Stream) internalRetransmit() error {
	s.streamSendLock.Lock()
	defer s.streamSendLock.Unlock()
	n := 0
	for _, seq := range s.streamAcks {
		n++
		spa, ok := s.streamAckMap[seq]
		if !ok {
			continue
		}
		delete(s.streamAckMap, seq)

		_, err := s.sendData(spa.flags, spa.off, spa.data)
		if err != nil {
			return err
		}
	}
	s.streamAcks = s.streamAcks[n:]
	return nil
}

func (s *Stream) internalReset() {
	s.internalReadReset()
	s.internalWriteReset()
}

func (s *Stream) internalReadReset() {
	util.AsyncNotify(s.readRstCh)
	atomic.StoreUint32(&s.streamRead, 0)
	atomic.StoreUint32(&s.streamSize, 0)
	s.streamSeqs = make(map[uint16]bool)
}

func (s *Stream) internalWriteReset() {
	util.AsyncNotify(s.writeRstCh)
	atomic.StoreUint32(&s.streamRttSeq, 0)
	atomic.StoreUint32(&s.streamRttSize, 0)
	s.streamAcks = s.streamAcks[len(s.streamAcks):]
	s.streamAckMap = make(map[uint16]streamPendingAck)
}

func (s *Stream) internalClose(sendFin bool) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sendFin {
		if _, err := s.write(0, cmdFIN); err != nil {
			return err
		}
	}
	s.closeOnce.Do(func() {
		close(s.die)
	})
	if err := s.Conn.Close(); err != nil {
		return err
	}
	s.wg.Wait()
	return nil
}
