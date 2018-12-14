package hash

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"github.com/hackform/governor"
	"golang.org/x/crypto/scrypt"
	"net/http"
	"strconv"
	"strings"
)

//////////
// Hash //
//////////

const (
	moduleID = "hash"
)

const (
	// 2016
	// attack 0.19s, 64MB
	// user 0.32s
	scryptID             = "s0"
	scryptHashLength     = 32
	scryptSaltLength     = 32
	scryptWorkFactor     = 65536
	scryptMemBlocksize   = 8
	scryptParallelFactor = 1
)

type (
	scryptConfig struct {
		workFactor     int
		memBlocksize   int
		parallelFactor int
	}
)

func (c scryptConfig) String() string {
	return strings.Join([]string{
		strconv.Itoa(c.workFactor),
		strconv.Itoa(c.memBlocksize),
		strconv.Itoa(c.parallelFactor),
	}, ",")
}

func decodeScryptParams(parameters string) (*scryptConfig, bool) {
	params := strings.Split(parameters, ",")
	if len(params) != 3 {
		return nil, false
	}
	workFactor, err := strconv.Atoi(params[0])
	if err != nil {
		return nil, false
	}
	memBlocksize, err := strconv.Atoi(params[1])
	if err != nil {
		return nil, false
	}
	parallelFactor, err := strconv.Atoi(params[2])
	if err != nil {
		return nil, false
	}
	return &scryptConfig{
		workFactor:     workFactor,
		memBlocksize:   memBlocksize,
		parallelFactor: parallelFactor,
	}, true
}

func scryptHash(key string, salt []byte, hashLength int, c scryptConfig) ([]byte, error) {
	return scrypt.Key([]byte(key), salt, c.workFactor, c.memBlocksize, c.parallelFactor, hashLength)
}

const (
	moduleIDHash = moduleID + ".Hash"
)

func newScrypt(key string) (string, *governor.Error) {
	salt := make([]byte, scryptSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", governor.NewError(moduleIDHash, err.Error(), 0, http.StatusInternalServerError)
	}
	c := scryptConfig{
		workFactor:     scryptWorkFactor,
		memBlocksize:   scryptMemBlocksize,
		parallelFactor: scryptParallelFactor,
	}
	hash, err := scryptHash(key, salt, scryptHashLength, c)
	if err != nil {
		return "", governor.NewError(moduleIDHash, err.Error(), 0, http.StatusInternalServerError)
	}

	b := strings.Builder{}
	b.WriteString("$")
	b.WriteString(scryptID)
	b.WriteString("$")
	b.WriteString(c.String())
	b.WriteString("$")
	b.WriteString(base64.RawURLEncoding.EncodeToString(salt))
	b.WriteString("$")
	b.WriteString(base64.RawURLEncoding.EncodeToString(hash))
	return b.String(), nil
}

// KDF returns a new hash for a given key
func KDF(key string) (string, *governor.Error) {
	return newScrypt(key)
}

func verifyScrypt(key string, b []string) bool {
	if len(b) != 4 || b[0] != scryptID {
		return false
	}
	config, ok := decodeScryptParams(b[1])
	if !ok {
		return false
	}
	salt, err := base64.RawURLEncoding.DecodeString(b[2])
	if err != nil {
		return false
	}
	hash, err := base64.RawURLEncoding.DecodeString(b[3])
	if err != nil {
		return false
	}
	res, err := scryptHash(key, salt, len(hash), *config)
	if err != nil {
		return false
	}
	return bytes.Equal(res, hash)
}

// VerifyKDF checks to see if the hash of the given key matches the provided keyhash
func VerifyKDF(key string, keyhash string) bool {
	b := strings.Split(strings.TrimLeft(keyhash, "$"), "$")
	switch b[0] {
	case scryptID:
		return verifyScrypt(key, b)
	default:
		return false
	}
}
