package main

import (
	"datagram-toolkit/crypto"
	"datagram-toolkit/example/shared"
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
	message := "Hello, world!"
	log.Infof("Sending to server: %s", message)
	if _, err := crypto.Write([]byte(message)); err != nil {
		return err
	}

	buf := make([]byte, 65535)
	n, err := crypto.Read(buf)
	if err != nil {
		return err
	}
	response := string(buf[:n])
	log.Infof("Received from server: %s", response)

	return nil
}
