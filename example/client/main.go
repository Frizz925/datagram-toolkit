package main

import (
	"crypto/rand"
	"datagram-toolkit/crypto"
	"datagram-toolkit/example/shared"
	"io"
	"net"
	"os"

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

	crypto := crypto.New(conn, crypto.DefaultConfig(aead))
	log.Infof("Sending to server, length: %d", len(payload))
	if _, err := crypto.Write(payload); err != nil {
		return err
	}

	buf := make([]byte, 65535)
	n, err := crypto.Read(buf)
	if err != nil {
		return err
	}
	log.Infof("Received from server, length: %d", n)

	return nil
}
