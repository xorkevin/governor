package uid

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"time"

	"xorkevin.dev/kerrors"
)

// ErrRand is returned when failing to read random bytes
var ErrRand errRand

type (
	errRand struct{}
)

func (e errRand) Error() string {
	return "Error reading rand"
}

var base64HexEncoding = base64.NewEncoding("-0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdefghijklmnopqrstuvwxyz").WithPadding(base64.NoPadding)

type (
	// UID is a time orderable universally unique identifier
	UID struct {
		u [16]byte
	}
)

// New creates a new [UID]
func New() (*UID, error) {
	u := &UID{}
	_, err := rand.Read(u.u[6:])
	if err != nil {
		return nil, kerrors.WithKind(err, ErrRand, "Failed reading crypto/rand")
	}
	now := uint64(time.Now().Round(0).UnixMilli())
	u.u[0] = byte(now)
	u.u[1] = byte(now >> 8)
	u.u[2] = byte(now >> 16)
	u.u[3] = byte(now >> 24)
	u.u[4] = byte(now >> 32)
	u.u[5] = byte(now >> 40)
	return u, nil
}

// Bytes returns the full raw bytes
func (u UID) Bytes() []byte {
	return u.u[:]
}

// Base64 returns the full raw bytes encoded in unpadded base64hex
func (u UID) Base64() string {
	return base64HexEncoding.EncodeToString(u.u[:])
}

type (
	// Snowflake is a short, time orderable unique identifier
	Snowflake uint64
)

// NewSnowflake returns a new [Snowflake] with a provided seq number
func NewSnowflake(seq uint32) Snowflake {
	now := uint64(time.Now().Round(0).UnixMilli())
	now = now << 24
	now |= (uint64(seq) & 0xffffff)
	return Snowflake(now)
}

// Base64 returns the full raw bytes encoded in unpadded base64hex
func (s Snowflake) Base64() string {
	var u [8]byte
	binary.BigEndian.PutUint64(u[:], uint64(s))
	return base64HexEncoding.EncodeToString(u[:])
}

// NewRandSnowflake returns a new [Snowflake] with random bytes for the seq
func NewRandSnowflake() (Snowflake, error) {
	var u [3]byte
	_, err := rand.Read(u[:])
	if err != nil {
		return 0, kerrors.WithKind(err, ErrRand, "Failed reading crypto/rand")
	}
	k := uint32(u[0])
	k |= uint32(u[1]) << 8
	k |= uint32(u[2]) << 16
	return NewSnowflake(k), nil
}

type (
	// Key is a secret key
	Key struct {
		u [32]byte
	}
)

// NewKey creates a new Key
func NewKey() (*Key, error) {
	u := &Key{}
	_, err := rand.Read(u.u[:])
	if err != nil {
		return nil, kerrors.WithKind(err, ErrRand, "Failed reading crypto/rand")
	}
	return u, nil
}

// Bytes returns the full raw bytes
func (u Key) Bytes() []byte {
	return u.u[:]
}

// Base64 returns the full raw bytes encoded in unpadded base64hex
func (u Key) Base64() string {
	return base64HexEncoding.EncodeToString(u.u[:])
}
