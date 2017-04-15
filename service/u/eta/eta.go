package eta

import (
	"bytes"
	"crypto/rand"
	"golang.org/x/crypto/scrypt"
)

//////////
// Hash //
//////////

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

const (
	// Latest holds the value of the latest version of eta
	Latest = 1
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

// Hash returns a new hash and salt for a given password
func Hash(password string, version int) (h, s []byte, v int, e error) {
	c := newConfig(version)
	salt := make([]byte, c.saltLength)
	_, err := rand.Read(salt)
	if err != nil {
		return []byte{}, salt, 0, err
	}
	hash, err := scrypt.Key([]byte(password), salt, c.workFactor, c.memBlocksize, c.parallelFactor, c.hashLength)
	return hash, salt, c.version, err
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
