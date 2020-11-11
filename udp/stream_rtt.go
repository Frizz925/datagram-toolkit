package udp

import (
	"datagram-toolkit/udp/protocol"
	"datagram-toolkit/util"
	"time"
)

func (s *Stream) rttRoutine() {
	defer s.wg.Done()
	// Ticker for timeout inflight acks
	tickDuration := 3 * time.Second
	ticker := time.NewTicker(tickDuration)
	defer ticker.Stop()
	inflight := false
	measureSeq := uint16(0)
	for {
		select {
		case seq := <-s.rttSend:
			if inflight {
				continue
			}
			inflight = true
			measureSeq = seq
			s.rttStats.UpdateSend()
			ticker.Reset(tickDuration)
		case seq := <-s.rttRecv:
			if !inflight || measureSeq != seq {
				continue
			}
			inflight = false
			s.rttStats.UpdateRecv()
			s.log("New RTT: %dms", s.rttStats.Smoothed()/time.Millisecond)
			util.AsyncNotify(s.retransmitNotify)
			util.AsyncNotify(s.ackNotify)
			ticker.Reset(tickDuration)
		case <-ticker.C:
			// Reset the measurement (timeout)
			inflight = false
		case <-s.die:
			return
		}
	}
}

func (s *Stream) measureSendRtt(seq uint16, flags uint8, cmd uint8) {
	if flags&protocol.FlagACK != 0 {
		return
	}
	switch cmd {
	case protocol.CmdSYN:
		fallthrough
	case protocol.CmdACK:
		fallthrough
	case protocol.CmdFIN:
		return
	}
	s.rttSend <- seq
}

func (s *Stream) measureRecvRtt(seq uint16, flags uint8) {
	s.rttRecv <- seq
}
