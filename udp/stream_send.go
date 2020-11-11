package udp

import (
	"datagram-toolkit/udp/protocol"
	"io"
	"math"
)

func (s *Stream) sendRoutine() {
	defer s.wg.Done()
	seq := uint16(0)
	maxSeq := uint16(math.MaxUint16)
	for {
		select {
		case req := <-s.sendCh:
			// Prevent overflow
			if seq >= maxSeq {
				seq = 0
			}
			seq++
			var res streamSendResult
			res.n, res.err = s.internalWrite(seq, req.flags, req.cmd, req.data)
			if len(req.head) > 0 {
				s.sendPool.Put(req.head)
			}
			req.result <- res
		case <-s.die:
			return
		}
	}
}

// Send handshake header followed with padding up to read buffer size.
// The padding is then used to determine the frame size from peer's side.
func (s *Stream) sendHandshake() error {
	hdr := protocol.NewHandshakeHdr(s.windowSize)
	buf := s.recvPool.Get()
	defer s.recvPool.Put(buf)
	copy(buf, hdr[:])
	_, err := s.write(0, protocol.CmdSYN, buf)
	return err
}

func (s *Stream) sendHandshakeAck(fsize, wsize uint32) error {
	ack := protocol.NewHandshakeAck(fsize, wsize)
	_, err := s.write(protocol.FlagACK, protocol.CmdSYN, ack[:])
	return err
}

func (s *Stream) sendData(flags uint8, offset int, b []byte) (int, error) {
	hdr := protocol.NewDataHdr(uint32(offset), uint32(len(b)))
	n, err := s.write(flags, protocol.CmdPSH, hdr[:], b)
	if err != nil {
		return 0, err
	}
	return n - len(hdr), nil
}

func (s *Stream) sendDataRst() error {
	_, err := s.write(protocol.FlagRST, protocol.CmdPSH)
	return err
}

func (s *Stream) sendClose() error {
	_, err := s.write(protocol.FlagFIN, protocol.CmdFIN)
	return err
}

func (s *Stream) writeAsync(flags, cmd uint8, body ...[]byte) <-chan streamSendResult {
	ch := make(chan streamSendResult, 1)
	req := streamSendRequest{
		flags:  flags,
		cmd:    cmd,
		result: ch,
	}
	if len(body) > 0 {
		req.head = s.sendPool.Get()
		n := 0
		for _, b := range body {
			n += copy(req.head[n:], b)
		}
		req.data = req.head[:n]
	}
	select {
	case s.sendCh <- req:
	case <-s.die:
		ch <- streamSendResult{err: io.EOF}
	}
	return ch
}

func (s *Stream) write(flags, cmd uint8, body ...[]byte) (int, error) {
	ch := s.writeAsync(flags, cmd, body...)
	select {
	case res := <-ch:
		return res.n, res.err
	case <-s.die:
		return 0, io.EOF
	}
}

func (s *Stream) internalWrite(seq uint16, flags, cmd uint8, b []byte) (int, error) {
	hdr := protocol.NewStreamHdr(seq, flags, cmd)
	if _, err := s.writer.Write(hdr[:]); err != nil {
		return 0, err
	}
	n, err := s.writer.Write(b)
	if err != nil {
		return 0, err
	}
	if err := s.writer.Flush(); err != nil {
		return 0, err
	}
	return n, nil
}
