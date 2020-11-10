package main

import (
	"crypto/cipher"
	"datagram-toolkit/crypto"
	"datagram-toolkit/example/shared"
	"datagram-toolkit/mux"
	"datagram-toolkit/netem"
	"datagram-toolkit/udp"
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
	ReadFragmentSize: 24,
	ReadReorderNth:   2,
	ReadDuplicateNth: 4,

	WriteFragmentSize: 24,
	WriteReorderNth:   2,
	WriteDuplicateNth: 4,
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
	l, err := udp.Listen("udp", laddr, udp.DefaultConfig())
	if err != nil {
		return err
	}
	log.WithField("addr", l.Addr()).Info("Server listening")

	// Start listening
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go listenRoutine(wg, l, aead)

	// Handle signals
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	log.Infof("Received signal %+v", <-ch)

	// Cleanup
	timer := time.NewTimer(3 * time.Second)
	done := make(chan error)
	go func() {
		if err := l.Close(); err != nil {
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
	logFields := logrus.Fields{
		"addr": l.Addr(),
	}
	if err := listen(wg, l, aead); err != nil && err != io.EOF {
		log.WithFields(logFields).Errorf("Listen error: %+v", err)
	} else {
		log.WithFields(logFields).Infof("Listen shutdown successfully")
	}
}

func listen(wg *sync.WaitGroup, l net.Listener, aead cipher.AEAD) error {
	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}
		log.WithField("addr", conn.RemoteAddr()).Info("Accepted new client from listener")
		wg.Add(1)
		go serveRoutine(wg, netem.New(conn, netemCfg), aead)
	}
}

func serveRoutine(wg *sync.WaitGroup, conn net.Conn, aead cipher.AEAD) {
	defer wg.Done()
	logFields := logrus.Fields{
		"addr": conn.RemoteAddr(),
	}
	if err := serve(conn, aead); err != nil && err != io.EOF {
		log.WithFields(logFields).Errorf("Serve error: %+v", err)
	} else {
		log.WithFields(logFields).Infof("Serve shutdown successfully")
	}
}

func serve(conn net.Conn, aead cipher.AEAD) error {
	defer conn.Close()
	deadline := time.Now().Add(15 * time.Second)
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}

	m := mux.Mux(conn, mux.DefaultConfig())
	defer m.Close()

	s, err := m.Accept()
	if err != nil {
		return err
	}
	defer s.Close()

	logFields := logrus.Fields{
		"addr":      conn.RemoteAddr(),
		"stream_id": s.ID(),
	}
	log.WithFields(logFields).Info("Accepted new stream from client")

	crypto := crypto.New(s, crypto.DefaultConfig(aead))
	buf := make([]byte, 65535)
	for {
		n, err := crypto.Read(buf)
		if err != nil {
			return err
		}
		log.WithFields(logFields).Infof("Received from client, length: %d", n)
		log.WithFields(logFields).Infof("Sending to client, length: %d", n)
		if _, err := crypto.Write(buf[:n]); err != nil {
			return err
		}
	}
}
