package main

import (
	"crypto/rand"
	"datagram-toolkit/crypto"
	"datagram-toolkit/example/shared"
	"datagram-toolkit/mux"
	"io"
	"net"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

var log = &logrus.Logger{
	Out:   os.Stdout,
	Level: logrus.DebugLevel,
	Formatter: &logrus.TextFormatter{
		FullTimestamp: true,
	},
}

func main() {
	if err := start(); err != nil {
		log.Fatal(err)
	}
}

func start() error {
	// Initialize the payload
	payload := make([]byte, 64)
	_, err := io.ReadFull(rand.Reader, payload)
	if err != nil {
		return err
	}

	// Initialize the AEAD
	aead := shared.CreateAEAD()

	// Connect to server
	raddr := shared.GetServerAddr()
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Create muxer
	m := mux.Mux(conn, mux.DefaultConfig())
	defer m.Close()

	// Open stream from muxer
	s, err := m.Open()
	if err != nil {
		return err
	}
	defer s.Close()

	// Send crypto frame to server
	crypto := crypto.New(s, crypto.DefaultConfig(aead))
	log.Infof("Sending to server, length: %d", len(payload))
	if _, err := crypto.Write(payload); err != nil {
		return err
	}

	// Set read deadline to 5 seconds
	deadline := time.Now().Add(5 * time.Second)
	if err := conn.SetReadDeadline(deadline); err != nil {
		return err
	}

	// Receive crypto frame from server
	buf := make([]byte, 65535)
	n, err := crypto.Read(buf)
	if err != nil {
		return err
	}
	log.Infof("Received from server, length: %d", n)

	return nil
}
