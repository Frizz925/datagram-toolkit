package udp

import (
	"datagram-toolkit/udp/protocol"
	"encoding/binary"
	"math"
)

const seqWindowRange = 16

func (s *Stream) recvRoutine() {
	defer s.wg.Done()
	buf := s.recvPool.Get()
	defer s.recvPool.Put(buf)
	var (
		seq uint16
		err error
	)
	maxSeq := uint16(math.MaxUint16)
	for {
		seq, err = s.internalRead(buf, seq)
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

func (s *Stream) readPacket(b []byte) (hdr protocol.StreamHdr, body []byte, err error) {
	var r int
	r, err = s.Conn.Read(b)
	if err != nil {
		return
	}
	w := copy(hdr[:], b[:r])
	return hdr, b[w:r], nil
}

func (s *Stream) readAck(b []byte) []uint16 {
	var ack protocol.AckHdr
	n := copy(ack[:], b)
	seqs := make([]uint16, ack.Len())
	for i := range seqs {
		if n > len(b)-2 {
			seqs = seqs[:i]
			break
		}
		seqs[i] = binary.BigEndian.Uint16(b[n:])
		n += 2
	}
	return seqs
}

func (s *Stream) readHandshake(b []byte) (hdr protocol.HandshakeHdr, fsize uint32) {
	n := copy(hdr[:], b)
	return hdr, uint32(len(b) - n)
}

func (s *Stream) readHandshakeAck(b []byte) (ack protocol.HandshakeAck) {
	copy(ack[:], b)
	return ack
}

func (s *Stream) readData(b []byte) (hdr protocol.DataHdr, data []byte) {
	n := copy(hdr[:], b)
	b = b[n:]
	n = int(hdr.Len())
	if n > len(b) {
		n = len(b)
	}
	return hdr, b[:n]
}

func (s *Stream) internalRead(buf []byte, seq uint16) (uint16, error) {
	hdr, body, err := s.readPacket(buf)
	if err != nil {
		return seq, err
	}

	nextSeq := hdr.Seq()
	// Sliding window mechanism, if the frame sequence is out of our window then drop it
	lowSeq := uint16(0)
	highSeq := uint16(math.MaxUint16)
	if seq < seqWindowRange {
		highSeq = seq + seqWindowRange
	} else if seq >= math.MaxUint16-seqWindowRange {
		lowSeq = seq - seqWindowRange
	} else {
		lowSeq = seq - seqWindowRange
		highSeq = seq + seqWindowRange
	}
	if nextSeq < lowSeq || nextSeq > highSeq {
		return seq, nil
	}
	// Next sequence should be higher than current sequence
	if nextSeq <= seq {
		nextSeq = seq
	}

	if h, ok := streamHandlers[hdr.Cmd()]; ok {
		s.log("Receiving frame: %s, Body(Length: %d)", hdr, len(body))
		if err := h(s, hdr.Flags(), body); err != nil {
			return nextSeq, err
		}
		s.maybeSendAck(hdr)
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
