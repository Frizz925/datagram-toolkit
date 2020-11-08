package udp

import (
	"datagram-toolkit/util"
	"encoding/binary"
	"sync/atomic"
)

func (s *Stream) handleReadError(err error) {
	s.readErr.Store(err)
	s.readErrCh <- err
}

func (s *Stream) handleHandshake(isAck bool) error {
	if isAck {
		var hack handshakeAck
		_, err := s.read(hack[:])
		if err != nil {
			return err
		}
		atomic.StoreUint32(&s.peerWindowSize, hack.WindowSize())
		atomic.StoreUint32(&s.frameSize, hack.FrameSize())
		s.handshakeCh <- nil
		util.AsyncNotifyErr(s.handshakeCh, nil)
		return nil
	}

	buf := s.buffers.Get()
	defer s.buffers.Put(buf)

	n, err := s.read(buf)
	if err != nil {
		return err
	}

	// Silently drop the handshake attempt if the (padded) frame size is too small
	n -= szStreamHdr + szStreamDataHdr
	if n < minStreamFrameSize {
		return nil
	}

	windowSize := binary.BigEndian.Uint32(buf)
	frameSize := uint32(n)
	atomic.StoreUint32(&s.peerWindowSize, windowSize)
	atomic.StoreUint32(&s.frameSize, frameSize)

	// Send back the ack frame
	go func(wsize, fsize uint32) {
		hack := newHandshakeAck(wsize, fsize)
		_, err := s.writeFrame(flagACK, cmdSYN, hack[:])
		util.AsyncNotifyErr(s.handshakeCh, err)
	}(windowSize, frameSize)

	return nil
}

func (s *Stream) handleReset(isAck bool) {
	if isAck {
		util.AsyncNotifyErr(s.resetCh, nil)
		s.internalReset()
		return
	}
	go func() {
		_, err := s.writeFrame(flagACK, cmdRST)
		util.AsyncNotifyErr(s.resetCh, err)
		s.internalReset()
	}()
}

func (s *Stream) handleData(flags uint8) error {
	s.readerLock.Lock()
	defer s.readerLock.Unlock()
	switch flags {
	case flagACK:
		var sda streamDataAck
		_, err := s.reader.Read(sda[:])
		if err != nil {
			return err
		}
		s.readAckCh <- sda
		return nil
	case flagFIN:
		var buf [4]byte
		_, err := s.reader.Read(buf[:])
		if err != nil {
			return err
		}
		size := int(binary.BigEndian.Uint32(buf[:]))
		s.readFinCh <- size
		return nil
	}

	var sdh streamDataHdr
	_, err := s.reader.Read(sdh[:])
	if err != nil {
		return err
	}

	buf := s.buffers.Get()
	n, err := s.reader.Read(buf[:sdh.Len()])
	if err != nil {
		return err
	}

	s.readCh <- streamReadResult{
		seq:  sdh.Seq(),
		off:  int(sdh.Off()),
		data: buf[:n],
		head: buf,
	}
	return nil
}
