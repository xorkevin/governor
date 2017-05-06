package hash

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"github.com/hackform/governor"
	"golang.org/x/crypto/scrypt"
	"net/http"
)

//////////
// Hash //
//////////

const (
	// Latest holds the value of the latest version
	vLatest       = 1
	versionLength = 4
	moduleID      = "hash"
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

const (
	moduleIDConfig = moduleID + ".newConfig"
)

func newConfig(version int) (*config, *governor.Error) {
	switch version {
	case v001.version:
		return v001, nil
	default:
		return nil, governor.NewError(moduleIDConfig, fmt.Sprintf("%d is not a valid version number", version), 0, http.StatusBadRequest)
	}
}

func (c *config) Version() int {
	return c.version
}

const (
	moduleIDHash = moduleID + ".Hash"
)

func shash(password string, salt []byte, c *config) ([]byte, error) {
	return scrypt.Key([]byte(password), salt, c.workFactor, c.memBlocksize, c.parallelFactor, c.hashLength)
}

// Hash returns a new hash and salt for a given password
func Hash(password string) ([]byte, *governor.Error) {
	c := latestConfig
	salt := make([]byte, c.saltLength)
	if _, err := rand.Read(salt); err != nil {
		return nil, governor.NewError(moduleIDHash, err.Error(), 0, http.StatusInternalServerError)
	}
	hash, errs := shash(password, salt, c)
	if errs != nil {
		return nil, governor.NewError(moduleIDHash, errs.Error(), 0, http.StatusInternalServerError)
	}
	b := bytes.Buffer{}
	if err := binary.Write(&b, binary.BigEndian, int32(vLatest)); err != nil {
		return nil, governor.NewError(moduleIDHash, err.Error(), 0, http.StatusInternalServerError)
	}
	b.Write(hash)
	b.Write(salt)
	return b.Bytes(), nil
}

// Verify checks to see if the hash of the given password and salt matches the provided passhash
func Verify(password string, passhash []byte) bool {
	// get the version
	if len(passhash) < versionLength {
		return false
	}
	var v int32
	if err := binary.Read(bytes.NewReader(passhash[0:versionLength]), binary.BigEndian, &v); err != nil {
		return false
	}
	c, err := newConfig(int(v))
	if err != nil {
		return false
	}

	if len(passhash) != versionLength+c.hashLength+c.saltLength {
		return false
	}

	// get the hash and salt
	hash := passhash[versionLength : versionLength+c.hashLength]
	salt := passhash[versionLength+c.hashLength:]
	dk, errs := shash(password, salt, c)
	if errs != nil {
		return false
	}
	return bytes.Equal(dk, hash)
}
