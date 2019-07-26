package uid

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"xorkevin.dev/governor/util/utime"
)

type (
	// UID is an identifier that can be initialized with a custom length composed of a user specified time, hash, and random bits
	UID struct {
		timebits,
		hashbits,
		randbits,
		size int
		u []byte
	}
)

// NewU creates a new UID without a hash input
func NewU(timesize, randsize int) (*UID, error) {
	return New(timesize, 0, randsize, nil)
}

// New creates a new UID
func New(timesize, hashsize, randsize int, input []byte) (*UID, error) {
	k := new(bytes.Buffer)

	if timesize > 0 {
		var t []byte
		timestamp, err := utime.Timestamp()
		if err != nil {
			return nil, err
		}
		if len(timestamp) < 1 {
			return nil, errors.New("No timestamp")
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
			return nil, errors.New("No hash input provided")
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

	return &UID{
		timebits: timesize,
		hashbits: hashsize,
		randbits: randsize,
		size:     timesize + hashsize + randsize,
		u:        k.Bytes(),
	}, nil
}

// FromBytes creates a new UID from an existing byte slice
func FromBytes(timesize, hashsize, randsize int, b []byte) (*UID, error) {
	size := timesize + hashsize + randsize
	if len(b) != size {
		return nil, errors.New(fmt.Sprintf("Byte slice length %d does not match defined sizes %d", len(b), size))
	}

	return &UID{
		timebits: timesize,
		hashbits: hashsize,
		randbits: randsize,
		size:     size,
		u:        b,
	}, nil
}

// FromBytesTRSplit creates a new UID from an existing byte slice with equal parts devoted to time and rand bytes
func FromBytesTRSplit(b []byte) (*UID, error) {
	if len(b)%2 != 0 {
		return nil, errors.New(fmt.Sprintf("byte slice length %d is not even", len(b)))
	}

	t := len(b) / 2

	return FromBytes(t, 0, t, b)
}

// FromBase64 creates a new UID from a base64 encoded string
func FromBase64(timeSize, hashSize, randomSize int, ustring string) (*UID, error) {
	b, err := base64.RawURLEncoding.DecodeString(ustring)
	if err != nil {
		return nil, err
	}

	return FromBytes(timeSize, hashSize, randomSize, b)
}

// FromBase64TRSplit creates a new UID from a base64 encoded string with equal parts devoted to time and rand bytes
func FromBase64TRSplit(ustring string) (*UID, error) {
	b, err := base64.RawURLEncoding.DecodeString(ustring)
	if err != nil {
		return nil, err
	}

	return FromBytesTRSplit(b)
}

// Bytes returns the full raw bytes of an UID
func (u *UID) Bytes() []byte {
	return u.u
}

// Time returns only the time bytes of an UID
func (u *UID) Time() []byte {
	return u.u[:u.timebits]
}

// Hash returns only the hash initialization bytes of an UID
func (u *UID) Hash() []byte {
	return u.u[u.timebits : u.timebits+u.hashbits]
}

// Rand returns only the random bytes of an UID
func (u *UID) Rand() []byte {
	return u.u[u.timebits+u.hashbits:]
}

// Base64 returns the full raw bytes of an UID encoded in standard padded base64
func (u *UID) Base64() string {
	return base64.RawURLEncoding.EncodeToString(u.u)
}

// TimeBase64 returns only the time bytes of an UID encoded in standard padded base64
func (u *UID) TimeBase64() string {
	return base64.RawURLEncoding.EncodeToString(u.Time())
}

// HashBase64 returns only the hash initialization bytes of an UID encoded in standard padded base64
func (u *UID) HashBase64() string {
	return base64.RawURLEncoding.EncodeToString(u.Hash())
}

// RandBase64 returns only the random bytes of an UID encoded in standard padded base64
func (u *UID) RandBase64() string {
	return base64.RawURLEncoding.EncodeToString(u.Rand())
}
