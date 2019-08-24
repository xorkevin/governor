package uid

import (
	"crypto/rand"
	"encoding/base64"
)

type (
	// UID is an identifier that can be initialized with a custom length composed of a user specified time, hash, and random bits
	UID struct {
		size int
		u    []byte
	}
)

// New creates a new UID
func New(size int) (*UID, error) {
	u := make([]byte, size)
	_, err := rand.Read(u)
	if err != nil {
		return nil, err
	}

	return &UID{
		size: size,
		u:    u,
	}, nil
}

// FromBytes creates a new UID from an existing byte slice
func FromBytes(b []byte) *UID {
	return &UID{
		size: len(b),
		u:    b,
	}
}

// FromBase64 creates a new UID from a base64 encoded string
func FromBase64(ustring string) (*UID, error) {
	b, err := base64.RawURLEncoding.DecodeString(ustring)
	if err != nil {
		return nil, err
	}
	return FromBytes(b), nil
}

// Bytes returns the full raw bytes of an UID
func (u *UID) Bytes() []byte {
	return u.u
}

// Base64 returns the full raw bytes of an UID encoded in standard padded base64
func (u *UID) Base64() string {
	return base64.RawURLEncoding.EncodeToString(u.u)
}
