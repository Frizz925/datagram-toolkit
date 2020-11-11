package udp

import (
	"container/heap"
	"datagram-toolkit/udp/protocol"
	"time"
)

func (s *Stream) retransmitRoutine() {
	defer s.wg.Done()
	// TODO: Set ticker accordingly to RTT estimator
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	pendingMap := make(map[uint16]streamRetransmitPending)
	for {
		select {
		case p := <-s.retransmitCh:
			if _, ok := pendingMap[p.seq]; ok {
				continue
			}
			pendingMap[p.seq] = p
			continue
		case seqs := <-s.retransmitAckCh:
			for _, seq := range seqs {
				p, ok := pendingMap[seq]
				if !ok {
					continue
				}
				if len(p.head) > 0 {
					s.sendPool.Put(p.head)
				}
				delete(pendingMap, seq)
			}
			s.log("Acknowledged frames: %+v", seqs)
			continue
		case <-s.retransmitNotify:
			ticker.Reset(s.rttStats.Smoothed() * 2 / 3)
			continue
		case <-ticker.C:
		case <-s.die:
			return
		}
		if len(pendingMap) <= 0 {
			continue
		}
		var pendings streamRetransmitPendingHeap
		for seq, p := range pendingMap {
			heap.Push(&pendings, p)
			delete(pendingMap, seq)
		}
		go s.internalRetransmit(pendings)
	}
}

func (s *Stream) internalRetransmit(pendings streamRetransmitPendingHeap) {
	var err error
	for pendings.Len() > 0 {
		p := heap.Pop(&pendings).(streamRetransmitPending)
		req := p.streamSendRequest
		if len(req.head) > 0 {
			_, err = s.write(req.flags, req.cmd, req.data)
			s.sendPool.Put(req.head)
		} else {
			_, err = s.write(req.flags, req.cmd)
		}
		if err != nil {
			break
		}
		s.log("Retransmitted frame: %d", p.seq)
	}
}

func (s *Stream) maybeRetransmit(seq uint16, req streamSendRequest) {
	// Do not retransmit ack frame
	if req.flags&protocol.FlagACK != 0 {
		return
	}
	// Filter out frames that doesn't need retransmission
	switch req.cmd {
	case protocol.CmdSYN:
		fallthrough
	case protocol.CmdFIN:
		fallthrough
	case protocol.CmdACK:
		return
	}
	s.retransmitCh <- streamRetransmitPending{
		seq:               seq,
		streamSendRequest: req,
	}
}
