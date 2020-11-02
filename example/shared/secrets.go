package shared

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"
	"io/ioutil"
	"os"
)

const KeyFilename = "shared.key"

func CreateAEAD() cipher.AEAD {
	// Read the shared key
	key, err := loadSharedKey(KeyFilename)
	if err != nil {
		panic(err)
	}
	// Initialize the AEAD
	cb, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}
	aead, err := cipher.NewGCM(cb)
	if err != nil {
		panic(err)
	}
	return aead
}

func loadSharedKey(filename string) ([]byte, error) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		if err := generateSharedKey(filename); err != nil {
			return nil, err
		}
	}
	return ioutil.ReadFile(filename)
}

func generateSharedKey(filename string) error {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return err
	}
	return ioutil.WriteFile(filename, key, 0600)
}
