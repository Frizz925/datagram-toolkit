package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	utilio "datagram-toolkit/util/io"
	"io"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCrypto(t *testing.T) {
	require := require.New(t)
	expected := []byte("This is the plaintext content")

	pr1, pw1 := io.Pipe()
	pr2, pw2 := io.Pipe()

	rwc1 := utilio.NewReadWriteCloser(pr1, pw2)
	defer rwc1.Close()
	rwc2 := utilio.NewReadWriteCloser(pr2, pw1)
	defer rwc2.Close()

	buf1 := make([]byte, 512)
	go io.CopyBuffer(rwc1, rwc2, buf1)
	buf2 := make([]byte, 512)
	go io.CopyBuffer(rwc2, rwc1, buf2)

	rand := rand.New(rand.NewSource(0))
	key := make([]byte, 32)
	_, err := io.ReadFull(rand, key)
	require.Nil(err)
	block, err := aes.NewCipher(key)
	require.Nil(err)
	aead, err := cipher.NewGCM(block)
	require.Nil(err)
	cfg := DefaultConfig(aead)

	conn1 := New(rwc1, cfg)
	w, err := conn1.Write(expected)
	require.Nil(err)
	require.Equal(len(expected), w)

	conn2 := New(rwc2, cfg)
	buf := make([]byte, 512)
	r, err := conn2.Read(buf)
	require.Nil(err)
	require.Equal(w, r)
	require.Equal(len(expected), r)

	actual := buf[:r]
	require.Equal(expected, actual)
}
