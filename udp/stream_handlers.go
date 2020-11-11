package udp

import (
	"datagram-toolkit/udp/protocol"
	"datagram-toolkit/util"
	"io"
	"sync/atomic"
)

type streamHandler func(s *Stream, flags uint8) error

var streamHandlers = map[uint8]streamHandler{
	protocol.CmdSYN: (*Stream).handleSYN,
	protocol.CmdFIN: (*Stream).handleFIN,
	protocol.CmdPSH: (*Stream).handlePSH,
}

func (s *Stream) handleSYN(flags uint8) error {
	switch flags {
	case protocol.FlagACK:
		ack, err := s.readHandshakeAck()
		if err != nil {
			return err
		}
		atomic.StoreUint32(&s.peerWindowSize, ack.WindowSize())
		atomic.StoreUint32(&s.frameSize, ack.FrameSize())
		util.AsyncNotify(s.handshakeNotify)
		return nil
	}
	buf := s.recvPool.Get()
	defer s.recvPool.Put(buf)

	var hdr protocol.HandshakeHdr
	n, err := s.reader.Read(buf)
	if err != nil {
		return err
	}
	copy(hdr[:], buf[:n])

	wsize := hdr.WindowSize()
	fsize := uint32(n - len(hdr))

	atomic.StoreUint32(&s.peerWindowSize, wsize)
	atomic.StoreUint32(&s.frameSize, fsize)

	if err := s.sendHandshakeAck(wsize, fsize); err != nil {
		return err
	}
	util.AsyncNotify(s.handshakeNotify)
	return nil
}

func (s *Stream) handleFIN(flags uint8) error {
	//nolint:errcheck
	go s.internalClose(false)
	return nil
}

func (s *Stream) handlePSH(flags uint8) error {
	switch flags {
	case protocol.FlagRST:
		return s.internalReset(false)
	}
	isFin := flags == protocol.FlagFIN

	var hdr protocol.DataHdr
	if _, err := s.reader.Read(hdr[:]); err != nil {
		return err
	}
	off := int(hdr.Off())
	size := int(hdr.Len())

	// Read the rest of the frame first before doing anything else
	buf := s.recvPool.Get()
	r, err := io.ReadFull(s.reader, buf[:size])
	if err != nil {
		return err
	}

	// Data offset larger than our window size, tell peer to reset stream
	wsize := int(atomic.LoadUint32(&s.windowSize))
	if off > wsize {
		return s.internalReset(true)
	}

	roff := int(atomic.LoadUint32(&s.recvOffset))
	// Ignore if this frame offset is lower than our read offset
	// (eg. due to frame retransmission)
	if off < roff {
		return nil
	}

	// Buffer the frame for read later
	buffer := s.recvBuffer.Load().([]streamRecvPending)
	pending := streamRecvPending{
		off:   off,
		data:  buf[:r],
		head:  buf,
		isFin: isFin,
	}
	if len(buffer) > 0 {
		// Insert the buffer in order according to their offset
		var (
			i int
			p streamRecvPending
		)
		for i, p = range buffer {
			if p.off > off {
				break
			}
		}
		head := append(buffer[:i], pending)
		head = append(head, buffer[i:]...)
		buffer = head
	} else {
		// Else just append to the tail
		buffer = append(buffer, pending)
	}

	// If the head of the buffer's offset doesn't match our read offset
	// then wait until the next frame with matching offset arrives
	// (eg. due to packet reordering)
	if buffer[0].off != roff {
		s.recvBuffer.Store(buffer)
		return nil
	}

	for len(buffer) > 0 {
		frame := buffer[0]
		buffer = buffer[1:]
		if frame.off < roff {
			// We check again for the offset in the buffered frame
			// If it's somehow lower than our read offset, then ignore it and read the next buffered frame
			continue
		} else if frame.off != roff {
			// Else if the next buffered frame somehow doesn't, match our read offset, then bail out
			// (eg. it still hasn't arrived until we started reading from buffer)
			break
		}
		n := copy(s.window[frame.off:], frame.data)
		roff += n
		s.recvPool.Put(frame.head)

		// We've reached the final frame, we can let the application read the data
		if frame.isFin {
			// Queue the result for read to application
			buf := make([]byte, roff)
			copy(buf, s.window)
			s.recvCh <- buf
			// Reset the stream
			return s.internalReset(false)
		}
	}
	s.recvBuffer.Store(buffer)
	atomic.StoreUint32(&s.recvOffset, uint32(roff))

	return nil
}
