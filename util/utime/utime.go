package utime

import (
	"bytes"
	"encoding/binary"
	"time"
)

// Timestamp creates a new 12 byte array from Unixtime. The first 8 bytes are
// seconds and next 4 bytes are nanoseconds
func Timestamp() ([]byte, error) {
	b := bytes.Buffer{}
	t := time.Now()
	if err := binary.Write(&b, binary.BigEndian, t.Unix()); err != nil {
		return nil, err
	}
	if err := binary.Write(&b, binary.BigEndian, int32(t.Nanosecond())); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
