package udp

import (
	"datagram-toolkit/udp/protocol"
	"sync/atomic"
	"time"
)

func (s *Stream) ackRoutine() {
	defer s.wg.Done()
	// TODO: Set ticker accordingly to RTT estimator
	ticker := time.NewTicker(125 * time.Millisecond)
	defer ticker.Stop()
	seqMap := make(map[uint16]bool)
	for {
		select {
		case seq := <-s.ackCh:
			fsize := int(atomic.LoadUint32(&s.frameSize))
			if fsize <= 0 {
				continue
			}
			if seqMap[seq] {
				continue
			}
			seqMap[seq] = true
			if len(seqMap) < fsize/2-1 {
				continue
			}
		case <-ticker.C:
		case <-s.die:
			return
		}
		if len(seqMap) <= 0 {
			continue
		}
		seqs := make([]uint16, len(seqMap))
		i := 0
		for seq := range seqMap {
			seqs[i] = seq
			delete(seqMap, seq)
			i++
		}
		go s.internalAck(seqs)
	}
}

func (s *Stream) internalAck(seqs []uint16) {
	s.log("Acking frames: %+v", seqs)
	s.sendAck(seqs)
	s.log("Acked frames: %+v", seqs)
}

func (s *Stream) maybeSendAck(hdr protocol.StreamHdr) {
	// If it's a frame with ACK flag then no need to do anything
	if hdr.Flags()&protocol.FlagACK != 0 {
		return
	}
	// Filter out frames that doesn't need acknowledgement
	switch hdr.Cmd() {
	case protocol.CmdSYN:
		fallthrough
	case protocol.CmdACK:
		fallthrough
	case protocol.CmdFIN:
		return
	}
	s.ackCh <- hdr.Seq()
}
