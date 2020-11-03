package muxer

import (
	"datagram-toolkit/util/errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUDPListener(t *testing.T) {
	require := require.New(t)
	expected := []byte("This is a message")

	laddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.Nil(err)
	ul, err := ListenUDP("udp", laddr, DefaultConfig())
	require.Nil(err)
	defer ul.Close()

	ch := make(chan error, 1)
	go serve(ul, ch)

	conn, err := net.DialUDP("udp", nil, ul.Addr().(*net.UDPAddr))
	require.Nil(err)
	defer conn.Close()

	w, err := conn.Write(expected)
	require.Nil(err)
	require.Equal(len(expected), w)

	buf := make([]byte, 512)
	r, err := conn.Read(buf)
	require.Nil(err)
	require.Equal(w, r)
	require.Equal(expected, buf[:r])

	client, err := ul.Open(conn.LocalAddr().(*net.UDPAddr))
	require.Nil(err)

	_, err = client.Read(nil)
	require.Equal(io.EOF, err)
	_, err = client.Write(nil)
	require.Equal(io.EOF, err)

	deadline := time.Now().Add(10 * time.Millisecond)
	require.Nil(client.SetDeadline(deadline))
	_, err = client.Read(buf)
	require.Equal(errors.ErrTimeout, err)

	require.Nil(ul.Close())
	require.Nil(<-ch)
}

func serve(l net.Listener, ch chan<- error) {
	defer close(ch)
	conn, err := l.Accept()
	if err != nil {
		ch <- err
		return
	}
	defer conn.Close()
	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	if err != nil {
		ch <- err
		return
	}
	if _, err := conn.Write(buf[:n]); err != nil {
		ch <- err
		return
	}
}
