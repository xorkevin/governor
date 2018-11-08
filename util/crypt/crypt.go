package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"github.com/hackform/governor"
	"golang.org/x/crypto/scrypt"
	"net/http"
)

const (
	scryptHashLength     = 32
	scryptSaltLength     = 32
	scryptWorkFactor     = 32768
	scryptMemBlocksize   = 8
	scryptParallelFactor = 1
	gcmNonceLength       = 12
)

const (
	moduleID = "crypt"
)

func Encrypt(key string, plaintext []byte) ([]byte, []byte, []byte, *governor.Error) {
	salt := make([]byte, scryptSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return nil, nil, nil, governor.NewError(moduleID, err.Error(), 0, http.StatusInternalServerError)
	}
	derived, err := scrypt.Key([]byte(key), salt, scryptWorkFactor, scryptMemBlocksize, scryptParallelFactor, scryptHashLength)
	if err != nil {
		return nil, nil, nil, governor.NewError(moduleID, err.Error(), 0, http.StatusInternalServerError)
	}

	block, err := aes.NewCipher(derived)
	if err != nil {
		return nil, nil, nil, governor.NewError(moduleID, err.Error(), 0, http.StatusInternalServerError)
	}

	nonce := make([]byte, gcmNonceLength)
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, nil, governor.NewError(moduleID, err.Error(), 0, http.StatusInternalServerError)
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, nil, governor.NewError(moduleID, err.Error(), 0, http.StatusInternalServerError)
	}

	ciphertext := aesgcm.Seal(nil, nonce, plaintext, nil)

	return ciphertext, salt, nonce, nil
}

func Decrypt(key string, salt, nonce, ciphertext []byte) ([]byte, *governor.Error) {
	derived, err := scrypt.Key([]byte(key), salt, scryptWorkFactor, scryptMemBlocksize, scryptParallelFactor, scryptHashLength)
	if err != nil {
		return nil, governor.NewError(moduleID, err.Error(), 0, http.StatusInternalServerError)
	}

	block, err := aes.NewCipher(derived)
	if err != nil {
		return nil, governor.NewError(moduleID, err.Error(), 0, http.StatusInternalServerError)
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, governor.NewError(moduleID, err.Error(), 0, http.StatusInternalServerError)
	}

	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, governor.NewError(moduleID, err.Error(), 0, http.StatusInternalServerError)
	}

	return plaintext, nil
}
