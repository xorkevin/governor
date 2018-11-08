package crypt

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_Crypt(t *testing.T) {
	assert := assert.New(t)
	key := "passkey"
	plaintext := []byte("Hello, World")

	ciphertext, salt, nonce, err := Encrypt(key, plaintext)
	assert.Nil(err, "Encrypt should be successful")

	decryptout, err := Decrypt(key, salt, nonce, ciphertext)
	assert.Nil(err, "Decrypt should be successful")
	assert.Equal(plaintext, decryptout, "decrypted message should be equal to original plaintext")
}
