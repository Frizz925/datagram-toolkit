package io

import "io"

type readWriter struct {
	io.Reader
	io.Writer
}

func NewReadWriter(r io.Reader, w io.Writer) io.ReadWriter {
	return readWriter{r, w}
}

type readWriteCloser struct {
	io.ReadCloser
	io.WriteCloser
}

func NewReadWriteCloser(rc io.ReadCloser, wc io.WriteCloser) io.ReadWriteCloser {
	return readWriteCloser{rc, wc}
}

func (rwc readWriteCloser) Close() error {
	if err := rwc.ReadCloser.Close(); err != nil {
		return err
	}
	if err := rwc.WriteCloser.Close(); err != nil {
		return err
	}
	return nil
}

func WriteFull(w io.Writer, buf []byte) error {
	written := 0
	for written < len(buf) {
		n, err := w.Write(buf[written:])
		if err != nil {
			return err
		}
		written += n
	}
	return nil
}

func ReadByte(r io.Reader) (byte, error) {
	buf := make([]byte, 1)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return 0, err
	}
	return buf[0], nil
}

func ReadBytes(r io.Reader, n int) ([]byte, error) {
	buf := make([]byte, n)
	_, err := io.ReadFull(r, buf)
	return buf, err
}
