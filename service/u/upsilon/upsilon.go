package upsilon

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"github.com/hackform/governor/service/u/tau"
)

///////////////////////
// Unique Identifier //
///////////////////////

type (
	// Upsilon is an identifier that can be initialized with a custom length composed of a user specified time, hash, and random bits
	Upsilon struct {
		timebits,
		hashbits,
		randbits,
		size int
		u []byte
	}
)

// NewU creates a new Upsilon without a hash input
func NewU(timesize, randsize int) (*Upsilon, error) {
	return New(timesize, 0, randsize, nil)
}

// New creates a new Upsilon
func New(timesize, hashsize, randsize int, input []byte) (*Upsilon, error) {
	k := new(bytes.Buffer)

	if timesize > 0 {
		var t []byte
		timestamp, err := tau.Timestamp()
		if err != nil {
			return nil, err
		}
		if len(timestamp) < 1 {
			return nil, fmt.Errorf("upsilon error: No timestamp")
		}
		t = make([]byte, timesize)
		l := len(timestamp) - timesize
		for i := 0; i < len(t); i++ {
			if l+i > -1 {
				t[i] = timestamp[l+i]
			}
		}
		k.Write(t)
	} else {
		timesize = 0
	}

	if hashsize > 0 {
		var h []byte
		if input == nil || len(input) < 1 {
			return nil, fmt.Errorf("upsilon error: No hash input provided")
		}
		h = make([]byte, hashsize)
		l := len(input) - hashsize
		for i := 0; i < len(h); i++ {
			if l+i > -1 {
				h[i] = input[l+i]
			}
		}
		k.Write(h)
	} else {
		hashsize = 0
	}

	if randsize > 0 {
		r := make([]byte, randsize)
		_, err := rand.Read(r)
		if err != nil {
			return nil, err
		}
		k.Write(r)
	} else {
		randsize = 0
	}

	return &Upsilon{
		timebits: timesize,
		hashbits: hashsize,
		randbits: randsize,
		size:     timesize + hashsize + randsize,
		u:        k.Bytes(),
	}, nil
}

// FromBytes creates a new Upsilon from an existing byte slice
func FromBytes(timesize, hashsize, randsize int, b []byte) (*Upsilon, error) {
	size := timesize + hashsize + randsize
	if len(b) != size {
		return nil, fmt.Errorf("upsilon error: byte slice length %d does not match defined sizes %d", len(b), size)
	}

	return &Upsilon{
		timebits: timesize,
		hashbits: hashsize,
		randbits: randsize,
		size:     size,
		u:        b,
	}, nil
}

// FromBase64 creates a new Upsilon from a base64 encoded string
func FromBase64(timeSize, hashSize, randomSize int, ustring string) (*Upsilon, error) {
	b, err := base64.URLEncoding.DecodeString(ustring)
	if err != nil {
		return nil, fmt.Errorf("upsilon error: %s", err)
	}

	return FromBytes(timeSize, hashSize, randomSize, b)
}

// Bytes returns the full raw bytes of an Upsilon
func (u *Upsilon) Bytes() []byte {
	return u.u
}

// Time returns only the time bytes of an Upsilon
func (u *Upsilon) Time() []byte {
	return u.u[:u.timebits]
}

// Hash returns only the hash initialization bytes of an Upsilon
func (u *Upsilon) Hash() []byte {
	return u.u[u.timebits : u.timebits+u.hashbits]
}

// Rand returns only the random bytes of an Upsilon
func (u *Upsilon) Rand() []byte {
	return u.u[u.timebits+u.hashbits:]
}

// Base64 returns the full raw bytes of an Upsilon encoded in standard padded base64
func (u *Upsilon) Base64() string {
	return base64.URLEncoding.EncodeToString(u.u)
}

// TimeBase64 returns only the time bytes of an Upsilon encoded in standard padded base64
func (u *Upsilon) TimeBase64() string {
	return base64.URLEncoding.EncodeToString(u.Time())
}

// HashBase64 returns only the hash initialization bytes of an Upsilon encoded in standard padded base64
func (u *Upsilon) HashBase64() string {
	return base64.URLEncoding.EncodeToString(u.Hash())
}

// RandBase64 returns only the random bytes of an Upsilon encoded in standard padded base64
func (u *Upsilon) RandBase64() string {
	return base64.URLEncoding.EncodeToString(u.Rand())
}
