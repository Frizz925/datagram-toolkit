package mux

import (
	"errors"
	"io"
	"net"
)

type UDPMuxer struct {
	conn *net.UDPConn
}

var _ Muxer = (*UDPMuxer)(nil)

func NewUDPMuxer(conn *net.UDPConn) *UDPMuxer {
	return &UDPMuxer{
		conn: conn,
	}
}

func (um *UDPMuxer) Accept() (io.ReadWriteCloser, error) {
	return um.AcceptStream()
}

func (um *UDPMuxer) AcceptStream() (Stream, error) {
	return nil, errors.New("not implemented")
}

func (um *UDPMuxer) Open() (io.ReadWriteCloser, error) {
	return um.OpenStream()
}

func (um *UDPMuxer) OpenStream() (Stream, error) {
	return nil, errors.New("not implemented")
}

func (um *UDPMuxer) Close() error {
	return errors.New("not implemented")
}
