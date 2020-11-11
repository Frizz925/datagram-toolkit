package udp

import (
	"datagram-toolkit/udp/protocol"
	"math"
)

func (s *Stream) recvRoutine() {
	defer s.wg.Done()
	var (
		seq uint16
		err error
	)
	maxSeq := uint16(math.MaxUint16)
	for {
		seq, err = s.internalRead(seq)
		if err != nil {
			s.handleReadError(err)
			return
		}
		// Prevent overflow
		if seq == maxSeq {
			seq = 0
		}
	}
}

func (s *Stream) readStreamHdr() (hdr protocol.StreamHdr, err error) {
	_, err = s.reader.Read(hdr[:])
	return hdr, err
}

func (s *Stream) readHandshakeAck() (ack protocol.HandshakeAck, err error) {
	_, err = s.reader.Read(ack[:])
	return ack, err
}

func (s *Stream) internalRead(seq uint16) (uint16, error) {
	hdr, err := s.readStreamHdr()
	if err != nil {
		return seq, err
	}
	nextSeq := hdr.Seq()
	// Sliding window mechanism, if the frame sequence is out of our window then drop it
	lowSeq := uint16(0)
	highSeq := uint16(math.MaxUint16)
	if seq < 8 {
		highSeq = seq + 16
	} else if seq >= math.MaxUint16-16 {
		lowSeq = seq - 8
	} else {
		lowSeq = seq - 8
		highSeq = seq + 16
	}
	if nextSeq < lowSeq || nextSeq > highSeq {
		return seq, nil
	}
	// Next sequence couldn't be lower than current sequence
	if nextSeq < seq {
		nextSeq = seq
	}
	if h, ok := streamHandlers[hdr.Cmd()]; ok {
		return nextSeq, h(s, hdr.Flags())
	}
	return nextSeq, nil
}

func (s *Stream) handleReadError(err error) {
	s.recvErr.Store(err)
	s.recvErrCh <- err
}

func (s *Stream) getReadError() error {
	if err, ok := s.recvErr.Load().(error); ok {
		return err
	}
	return nil
}
