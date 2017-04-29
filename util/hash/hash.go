package hash

import (
	"bytes"
	"crypto/rand"
	"github.com/hackform/governor"
	"golang.org/x/crypto/scrypt"
	"net/http"
)

//////////
// Hash //
//////////

const (
	// Latest holds the value of the latest version
	Latest   = 1
	moduleID = "hash"
)

type (
	config struct {
		version,
		hashLength,
		saltLength,
		workFactor,
		memBlocksize,
		parallelFactor int
	}
)

var (
	v001 = &config{
		version:        1,
		hashLength:     64,
		saltLength:     64,
		workFactor:     16384,
		memBlocksize:   8,
		parallelFactor: 1,
	}

	latestConfig = v001
)

func newConfig(version int) *config {
	switch version {
	case v001.version:
		return v001
	default:
		return latestConfig
	}
}

func (c *config) Version() int {
	return c.version
}

const (
	moduleIDHash = moduleID + ".Hash"
)

// Hash returns a new hash and salt for a given password
func Hash(password string, version int) (h, s []byte, v int, e *governor.Error) {
	c := newConfig(version)
	salt := make([]byte, c.saltLength)
	_, err := rand.Read(salt)
	if err != nil {
		return nil, nil, 0, governor.NewError(moduleIDHash, err.Error(), 0, http.StatusInternalServerError)
	}
	hash, err := scrypt.Key([]byte(password), salt, c.workFactor, c.memBlocksize, c.parallelFactor, c.hashLength)
	if err != nil {
		return nil, nil, 0, governor.NewError(moduleIDHash, err.Error(), 0, http.StatusInternalServerError)
	}
	return hash, salt, c.version, nil
}

// Verify checks to see if the hash of the given password and salt matches the provided passhash
func Verify(password string, salt, passhash []byte, version int) bool {
	c := newConfig(version)
	dk, err := scrypt.Key([]byte(password), salt, c.workFactor, c.memBlocksize, c.parallelFactor, c.hashLength)
	if err != nil {
		return false
	}
	return bytes.Equal(dk, passhash)
}
