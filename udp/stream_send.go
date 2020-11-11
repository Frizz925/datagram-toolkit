package udp

import (
	"datagram-toolkit/udp/protocol"
	"encoding/binary"
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
			if res.err == nil {
				s.maybeRetransmit(seq, req)
			} else if len(req.head) > 0 {
				s.sendPool.Put(req.head)
			}
			req.result <- res
		case <-s.die:
			return
		}
	}
}

func (s *Stream) sendAck(seqs []uint16) {
	buf := s.sendPool.Get()
	defer s.sendPool.Put(buf)
	hdr := protocol.NewAckHdr(uint16(len(seqs)))
	n := copy(buf, hdr[:])
	for _, seq := range seqs {
		binary.BigEndian.PutUint16(buf[n:], seq)
		n += 2
	}
	//nolint:errcheck
	s.write(protocol.FlagACK, protocol.CmdACK, buf[:n])
}

// Send handshake header followed with padding up to read buffer size.
// The padding is then used to determine the frame size from peer's side.
func (s *Stream) sendHandshake() error {
	hdr := protocol.NewHandshakeHdr(s.windowSize)
	// We use recv pool instead because it is used to determine
	// our frame size to our peer, which also depends on our read buffer size
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
	s.sendCh <- req
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
	buf := s.sendPool.Get()
	defer s.sendPool.Put(buf)
	hdr := protocol.NewStreamHdr(seq, flags, cmd)
	n := copy(buf, hdr[:])
	n += copy(buf[n:], b)
	if n > len(buf) {
		n = len(buf)
	}
	s.log("Sending frame: %s, Body(Length: %d)", hdr, len(b))
	w, err := s.Conn.Write(buf[:n])
	if err != nil {
		return 0, err
	}
	return w - len(hdr), nil
}
