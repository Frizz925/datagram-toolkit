package udp

import "time"

func (s *Stream) readRoutine() {
	defer func() {
		s.wg.Done()
		s.logger.Printf("%p: Read routine done", s)
	}()
	var (
		seq uint16
		err error
	)
	for {
		seq, err = s.internalRead(seq)
		if err != nil {
			s.handleReadError(err)
			return
		}
	}
}

func (s *Stream) writeRoutine() {
	defer func() {
		s.wg.Done()
		s.logger.Printf("%p: Write routine done", s)
	}()
	for {
		select {
		case req := <-s.writeCh:
			var res streamWriteResult
			res.n, res.err = s.internalWrite(req)
			req.result <- res
		case <-s.die:
			return
		}
	}
}

func (s *Stream) ackRoutine() {
	defer func() {
		s.wg.Done()
		s.logger.Printf("%p: Ack routine done", s)
	}()
}

func (s *Stream) retransmitRoutine() {
	defer func() {
		s.wg.Done()
		s.logger.Printf("%p: Retransmit routine done", s)
	}()
	ticker := time.NewTicker(1e+6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			err := s.internalRetransmit()
			if err != nil {
				s.handleRetransmitError(err)
				return
			}
		case <-s.retransmitNotify:
			ticker.Reset(s.rttStats.Smoothed())
		case <-s.die:
			return
		}
	}
}
