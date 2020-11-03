package main

import (
	"crypto/cipher"
	"datagram-toolkit/crypto"
	"datagram-toolkit/example/shared"
	"datagram-toolkit/muxer"
	"datagram-toolkit/netem"
	"errors"
	"io"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
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

var netemCfg = netem.Config{
	Backlog:           5,
	ReadFragmentSize:  24,
	WriteFragmentSize: 24,
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
	timer := time.NewTimer(3 * time.Second)
	done := make(chan error)
	go func() {
		if err := ul.Close(); err != nil {
			done <- err
			return
		}
		wg.Wait()
		close(done)
	}()

	select {
	case err := <-done:
		return err
	case <-timer.C:
		return errors.New("timeout while shutting down")
	}
}

func listenRoutine(wg *sync.WaitGroup, l net.Listener, aead cipher.AEAD) {
	defer wg.Done()
	if err := listen(wg, l, aead); err != nil && err != io.EOF {
		log.Errorf("Listen error: %+v", err)
	} else {
		log.Infof("Listen shutdown successfully")
	}
}

func listen(wg *sync.WaitGroup, l net.Listener, aead cipher.AEAD) error {
	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}
		ne := netem.New(conn, netemCfg)
		wg.Add(1)
		go serveRoutine(wg, ne, aead)
	}
}

func serveRoutine(wg *sync.WaitGroup, conn net.Conn, aead cipher.AEAD) {
	defer wg.Done()
	if err := serve(conn, aead); err != nil && err != io.EOF {
		log.Errorf("Serve error: %+v", err)
	} else {
		log.Infof("Serve shutdown successfully")
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
		log.Infof("Received from client %s, length: %d", addr, n)
		log.Infof("Sending to client %s, length: %d", addr, n)
		if _, err := crypto.Write(buf[:n]); err != nil {
			return err
		}
	}
}
