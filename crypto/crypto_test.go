package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	utilio "datagram-toolkit/util/io"
	"io"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCrypto(t *testing.T) {
	var err error
	require := require.New(t)
	expectedLen := 64
	expected := make([]byte, expectedLen)
	rand := rand.New(rand.NewSource(0))

	_, err = io.ReadFull(rand, expected)
	require.Nil(err)

	pr1, pw1 := io.Pipe()
	pr2, pw2 := io.Pipe()

	rwc1 := utilio.NewReadWriteCloser(pr1, pw2)
	defer rwc1.Close()
	rwc2 := utilio.NewReadWriteCloser(pr2, pw1)
	defer rwc2.Close()

	buf1 := make([]byte, 512)
	// nolint:errcheck
	go io.CopyBuffer(rwc1, rwc2, buf1)
	buf2 := make([]byte, 512)
	// nolint:errcheck
	go io.CopyBuffer(rwc2, rwc1, buf2)

	key := make([]byte, 32)
	_, err = io.ReadFull(rand, key)
	require.Nil(err)
	block, err := aes.NewCipher(key)
	require.Nil(err)
	aead, err := cipher.NewGCM(block)
	require.Nil(err)

	cfg1 := DefaultConfig(aead)
	cfg2 := Config{AEAD: aead}

	conn1 := New(rwc1, cfg1)
	w, err := conn1.Write(expected)
	require.Nil(err)
	require.Equal(len(expected), w)

	conn2 := New(rwc2, cfg2)
	b := make([]byte, 16)
	buf := bytes.Buffer{}
	for buf.Len() < expectedLen {
		n, err := conn2.Read(b)
		require.Nil(err)
		buf.Write(b[:n])
	}
	require.Equal(w, buf.Len())

	actual := buf.Bytes()
	require.Equal(expected, actual)
}
