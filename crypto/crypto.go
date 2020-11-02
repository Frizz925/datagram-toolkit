package crypto

import (
	"bufio"
	"bytes"
	"crypto/cipher"
	"datagram-toolkit/frame"
	"errors"
	"io"
	"sync"
)

const (
	defaultBufferSize = 65535
	minBufferSize     = 512
)

var ErrNotImplemented = errors.New("not implemented")

type Config struct {
	cipher.AEAD
	NonceReadWriter
	BufferSize int
}

func DefaultConfig(aead cipher.AEAD) Config {
	return Config{
		BufferSize: defaultBufferSize,
		AEAD:       aead,
		NonceReadWriter: &sequentialNonceReadWriter{
			size: aead.NonceSize(),
		},
	}
}

type Crypto struct {
	cipher.AEAD
	NonceReadWriter

	reader *bufio.Reader
	writer *bufio.Writer

	buffer *bytes.Buffer
	mu     sync.Mutex
}

func New(rw io.ReadWriter, cfg Config) *Crypto {
	if cfg.BufferSize < minBufferSize {
		cfg.BufferSize = minBufferSize
	}
	return &Crypto{
		AEAD:            cfg.AEAD,
		NonceReadWriter: cfg.NonceReadWriter,
		reader:          bufio.NewReaderSize(rw, cfg.BufferSize),
		writer:          bufio.NewWriterSize(rw, cfg.BufferSize),
		buffer:          bytes.NewBuffer(make([]byte, 0, cfg.BufferSize)),
	}
}

func (c *Crypto) Read(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.buffer.Len() > len(b) {
		return c.buffer.Read(b)
	}
	// Read the nonce
	nonce, err := c.ReadNonce(c.reader)
	if err != nil {
		return 0, err
	}
	// Read the ciphertext
	ciphertext, err := frame.ReadRaw(c.reader)
	if err != nil {
		return 0, err
	}
	// Decrypt to plaintext
	plaintext, err := c.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return 0, err
	}
	// Write to internal buffer
	c.buffer.Write(plaintext)
	return c.buffer.Read(b)
}

func (c *Crypto) Write(b []byte) (int, error) {
	n := len(b)
	buf := &bytes.Buffer{}
	// Write the nonce to buffer
	if err := c.WriteNonce(buf); err != nil {
		return 0, err
	}
	nonce := buf.Bytes()
	// Encrypt content to ciphertext
	ciphertext := c.Seal(nil, nonce, b, nil)
	if err := frame.WriteRaw(buf, ciphertext); err != nil {
		return 0, err
	}
	// Write to the underlying writer from buffer
	if _, err := buf.WriteTo(c.writer); err != nil {
		return 0, err
	}
	return n, c.writer.Flush()
}
