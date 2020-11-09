package udp

import "time"

func (s *Stream) readRoutine() {
	defer func() {
		s.wg.Done()
		s.logger.Printf("%p: Read routine done", s)
	}()
	for {
		err := s.internalRead()
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

func (s *Stream) retransmitRoutine() {
	defer func() {
		s.wg.Done()
		s.logger.Printf("%p: Retransmit routine done", s)
	}()
	for {
		select {
		case <-s.die:
			return
		default:
		}
		time.Sleep(s.rttStats.Smoothed())
		err := s.internalRetransmit()
		if err != nil {
			s.handleRetransmitError(err)
		}
	}
}
