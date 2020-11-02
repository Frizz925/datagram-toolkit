package main

import (
	"crypto/cipher"
	"datagram-toolkit/crypto"
	"datagram-toolkit/example/shared"
	"datagram-toolkit/muxer"
	"io"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

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

	// Initialize the listener
	laddr := shared.GetServerAddr()
	ul, err := muxer.ListenUDP("udp", laddr, muxer.DefaultConfig())
	if err != nil {
		return err
	}
	log.Infof("Server listening at %s", ul.Addr())

	// Start listening
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go listenRoutine(wg, ul, aead)

	// Handle signals
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	log.Infof("Received signal %+v", <-ch)

	// Cleanup
	ul.Close()
	wg.Wait()
	return nil
}

func listenRoutine(wg *sync.WaitGroup, l net.Listener, aead cipher.AEAD) {
	defer wg.Done()
	if err := listen(wg, l, aead); err != nil && err != io.EOF {
		log.Errorf("Listen error: %+v", err)
	}
}

func listen(wg *sync.WaitGroup, l net.Listener, aead cipher.AEAD) error {
	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}
		wg.Add(1)
		go serveRoutine(wg, conn, aead)
	}
}

func serveRoutine(wg *sync.WaitGroup, conn net.Conn, aead cipher.AEAD) {
	defer wg.Done()
	if err := serve(conn, aead); err != nil && err != io.EOF {
		log.Errorf("Serve error: %+v", err)
	}
}

func serve(conn net.Conn, aead cipher.AEAD) error {
	defer conn.Close()
	addr := conn.RemoteAddr()
	crypto := crypto.New(conn, crypto.DefaultConfig(aead))
	buf := make([]byte, 65535)
	for {
		n, err := crypto.Read(buf)
		if err != nil {
			return err
		}
		msg := buf[:n]
		log.Infof("Received from client %s: %s", addr, string(msg))
		log.Infof("Sending to client %s: %s", addr, string(msg))
		if _, err := crypto.Write(msg); err != nil {
			return err
		}
	}
}
