package uid

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"strings"
	"time"

	"xorkevin.dev/kerrors"
)

type (
	// UID is an identifier that can be initialized with a custom length composed of a user specified time, hash, and random bits
	UID struct {
		u []byte
	}
)

var (
	// ErrorRand is returned when failing to read random bytes
	ErrorRand errorRand
	// ErrorInvalidUID is returned on an invalid uid
	ErrorInvalidUID errorInvalidUID
)

type (
	errorRand       struct{}
	errorInvalidUID struct{}
)

func (e errorRand) Error() string {
	return "Error reading rand"
}

func (e errorInvalidUID) Error() string {
	return "Invalid uid"
}

// New creates a new UID
func New(size int) (*UID, error) {
	u := make([]byte, size)
	_, err := rand.Read(u)
	if err != nil {
		return nil, kerrors.WithKind(err, ErrorRand, "Failed reading crypto/rand")
	}

	return &UID{
		u: u,
	}, nil
}

// FromBytes creates a new UID from an existing byte slice
func FromBytes(b []byte) *UID {
	return &UID{
		u: b,
	}
}

// FromBase64 creates a new UID from a base64 encoded string
func FromBase64(ustring string) (*UID, error) {
	b, err := base64.RawURLEncoding.DecodeString(ustring)
	if err != nil {
		return nil, kerrors.WithKind(err, ErrorInvalidUID, "Invalid uid string")
	}
	return FromBytes(b), nil
}

// Bytes returns the full raw bytes of an UID
func (u *UID) Bytes() []byte {
	return u.u
}

// Base64 returns the full raw bytes of an UID encoded in unpadded base64url
func (u *UID) Base64() string {
	return base64.RawURLEncoding.EncodeToString(u.u)
}

type (
	// Snowflake is a uid approximately sortable by time
	Snowflake struct {
		u []byte
	}
)

const (
	timeSize = 8
)

// NewSnowflake creates a new snowflake uid
func NewSnowflake(randsize int) (*Snowflake, error) {
	u := make([]byte, timeSize+randsize)
	now := uint64(time.Now().Round(0).UnixMilli())
	binary.BigEndian.PutUint64(u[:timeSize], now)
	_, err := rand.Read(u[timeSize:])
	if err != nil {
		return nil, kerrors.WithKind(err, ErrorRand, "Failed reading crypto/rand")
	}
	return &Snowflake{
		u: u,
	}, nil
}

// Bytes returns the full raw bytes of a snowflake
func (s *Snowflake) Bytes() []byte {
	return s.u
}

var (
	base32RawHexEncoding = base32.HexEncoding.WithPadding(base32.NoPadding)
)

// Base32 returns the full raw bytes of a snowflake in unpadded base32hex
func (s *Snowflake) Base32() string {
	return strings.ToLower(base32RawHexEncoding.EncodeToString(s.u))
}

const (
	reqIDUnusedTimeSize    = 3
	reqIDTimeSize          = 5
	reqIDTotalTimeSize     = reqIDUnusedTimeSize + reqIDTimeSize
	reqIDCounterSize       = 3
	reqIDUnusedCounterSize = 1
	reqIDTotalCounterSize  = reqIDCounterSize + reqIDUnusedCounterSize
	reqIDSize              = reqIDTimeSize + reqIDCounterSize
	reqIDCounterShift      = 8 * reqIDUnusedCounterSize
)

func ReqID(count uint32) string {
	// id looks like:
	// reqIDUnusedTimeSize | reqIDTimeSize | reqIDCounterSize | reqIDUnusedCounterSize
	b := [reqIDTotalTimeSize + reqIDTotalCounterSize]byte{}
	now := uint64(time.Now().Round(0).UnixMilli())
	binary.BigEndian.PutUint64(b[:reqIDTotalTimeSize], now)
	binary.BigEndian.PutUint32(b[reqIDTotalTimeSize:], count<<reqIDCounterShift)
	return strings.ToLower(base32RawHexEncoding.EncodeToString(b[reqIDUnusedTimeSize : reqIDUnusedTimeSize+reqIDSize]))
}
