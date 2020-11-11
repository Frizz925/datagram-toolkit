package udp

import (
	"container/heap"
	"datagram-toolkit/udp/protocol"
	"datagram-toolkit/util"
	"sync/atomic"
)

type streamHandler func(s *Stream, flags uint8, b []byte) error

var streamHandlers = map[uint8]streamHandler{
	protocol.CmdSYN: (*Stream).handleSYN,
	protocol.CmdACK: (*Stream).handleACK,
	protocol.CmdFIN: (*Stream).handleFIN,
	protocol.CmdPSH: (*Stream).handlePSH,
}

func (s *Stream) handleSYN(flags uint8, b []byte) error {
	switch flags {
	case protocol.FlagACK:
		ack := s.readHandshakeAck(b)
		atomic.StoreUint32(&s.peerWindowSize, ack.WindowSize())
		atomic.StoreUint32(&s.frameSize, ack.FrameSize())
		util.AsyncNotify(s.handshakeNotify)
		return nil
	}
	buf := s.recvPool.Get()
	defer s.recvPool.Put(buf)

	hdr, fsize := s.readHandshake(b)
	wsize := hdr.WindowSize()

	atomic.StoreUint32(&s.peerWindowSize, wsize)
	atomic.StoreUint32(&s.frameSize, fsize)

	if err := s.sendHandshakeAck(wsize, fsize); err != nil {
		return err
	}
	util.AsyncNotify(s.handshakeNotify)
	return nil
}

func (s *Stream) handleACK(_ uint8, b []byte) error {
	seqs := s.readAck(b)
	if len(seqs) > 0 {
		s.retransmitAckCh <- s.readAck(b)
	}
	return nil
}

func (s *Stream) handleFIN(flags uint8, b []byte) error {
	// Unnecessary check but eh
	if flags == 0 && len(b) == 0 {
		//nolint:errcheck
		go s.internalClose(false)
	}
	return nil
}

func (s *Stream) handlePSH(flags uint8, b []byte) error {
	switch flags {
	case protocol.FlagRST:
		return s.internalReset(false)
	}
	isFin := flags == protocol.FlagFIN

	var hdr protocol.DataHdr
	hdr, data := s.readData(b)
	off := int(hdr.Off())

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

	buf := s.recvPool.Get()
	r := copy(buf, data)

	// Buffer the frame for read later
	buffer := s.recvBuffer.Load().(streamRecvPendingHeap)
	heap.Push(&buffer, streamRecvPending{
		off:   off,
		data:  buf[:r],
		head:  buf,
		isFin: isFin,
	})
	head := heap.Pop(&buffer).(streamRecvPending)

	// If the head of the buffer's offset doesn't match our read offset
	// then wait until the next frame with matching offset arrives
	// (eg. due to packet reordering)
	if head.off != roff {
		heap.Push(&buffer, head)
		s.recvBuffer.Store(buffer)
		return nil
	}
	heap.Push(&buffer, head)

	for buffer.Len() > 0 {
		head := heap.Pop(&buffer).(streamRecvPending)
		if head.off > roff {
			// We check again for the offset in the buffered frame
			// If the next buffered frame is higher than our read offset, then return it to the heap
			// (eg. it still hasn't arrived until we started reading from buffer)
			heap.Push(&buffer, head)
			break
		} else if head.off < roff {
			// If it's somehow lower than our read offset, then ignore it and read the next buffered frame
			s.recvPool.Put(head.head)
			continue
		}
		n := copy(s.window[head.off:], head.data)
		roff += n
		s.recvPool.Put(head.head)

		// We've reached the final frame, we can let the application read the data
		if head.isFin {
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
