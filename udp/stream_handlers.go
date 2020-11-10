package udp

import (
	"datagram-toolkit/util"
	"encoding/binary"
	"sync/atomic"
)

func (s *Stream) handleReadError(err error) {
	s.writeAsync(0, cmdFIN)
	s.readErr.Store(err)
	s.readErrCh <- err
}

func (s *Stream) handleRetransmitError(err error) {
	// Ignore retransmission errors for now until we find a better way to deal with them
}

func (s *Stream) handleHandshake(isAck bool) error {
	if isAck {
		var hack handshakeAck
		_, err := s.reader.Read(hack[:])
		if err != nil {
			return err
		}
		s.rttStats.UpdateRecv()

		atomic.StoreUint32(&s.peerWindowSize, hack.WindowSize())
		atomic.StoreUint32(&s.frameSize, hack.FrameSize())
		util.AsyncNotifyErr(s.handshakeCh, nil)

		return nil
	}

	buf := s.buffers.Get()
	defer s.buffers.Put(buf)

	n, err := s.reader.Read(buf)
	if err != nil {
		return err
	}

	// Silently drop the handshake attempt if the (padded) frame size is too small
	n -= szStreamDataHdr
	if n < minStreamFrameSize {
		return nil
	}

	wsize := binary.BigEndian.Uint32(buf)
	fsize := uint32(n)
	atomic.StoreUint32(&s.peerWindowSize, wsize)
	atomic.StoreUint32(&s.frameSize, fsize)

	// Send back the ack frame
	hack := newHandshakeAck(wsize, fsize)
	ch := s.writeAsync(flagACK, cmdSYN, hack[:])
	go func() {
		s.handshakeCh <- (<-ch).err
	}()

	return nil
}

func (s *Stream) handleReset(isAck bool) {
	if isAck {
		util.AsyncNotifyErr(s.resetCh, nil)
		return
	}
	go func() {
		_, err := s.write(flagACK, cmdRST)
		util.AsyncNotifyErr(s.resetCh, err)
		s.internalReset()
	}()
}

func (s *Stream) handleData(flags uint8) error {
	// Handle stream ACK frames
	// Stream frame flags are mutually exclusive, so we just use equal operators
	if flags == flagACK {
		return s.handleDataAck()
	}

	var sdh streamDataHdr
	if _, err := s.reader.Read(sdh[:]); err != nil {
		return err
	}
	// If frame has FIN flag, then this should be the last stream frame
	if flags == cmdFIN {
		size := sdh.Off() + sdh.Len()
		atomic.StoreUint32(&s.streamSize, size)
	}
	off := int(sdh.Off())
	length := int(sdh.Len())

	// Read the stream data
	buf := s.buffers.Get()
	defer s.buffers.Put(buf)
	n, err := s.reader.Read(buf[:length])
	if err != nil {
		return err
	}

	// Copy the stream data into our window buffer
	nn := copy(s.window[off:], buf[:n])

	read := atomic.AddUint32(&s.streamRead, uint32(nn))
	size := atomic.LoadUint32(&s.streamSize)
	// All frames have been read, put the buffer into backlog
	if size > 0 && read >= size {
		buf := make([]byte, read)
		copy(buf, s.window)
		s.readCh <- buf
		s.internalReset()
	}

	return nil
}

func (s *Stream) handleDataAck() error {
	var buf [2]byte
	if _, err := s.reader.Read(buf[:]); err != nil {
		return err
	}
	seq := binary.BigEndian.Uint16(buf[:])

	s.streamSendLock.Lock()
	defer s.streamSendLock.Unlock()
	if _, ok := s.streamAckMap[seq]; !ok {
		return nil
	}
	delete(s.streamAckMap, seq)

	rttSeq := atomic.LoadUint32(&s.streamRttSeq)
	if rttSeq == uint32(seq) {
		s.rttStats.UpdateRecv()
	}
	util.AsyncNotify(s.retransmitNotify)

	return nil
}
