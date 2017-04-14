package tau

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"
)

//////////
// Time //
//////////

// Timestamp creates a new 12 byte array from Unixtime
// first 8 bytes are seconds and next 4 bytes are nanoseconds
func Timestamp() ([]byte, error) {
	b := new(bytes.Buffer)
	t := time.Now()
	if err := binary.Write(b, binary.BigEndian, t.Unix()); err != nil {
		return nil, fmt.Errorf("tau error: %s", err.Error())
	}
	if err := binary.Write(b, binary.BigEndian, int32(t.Nanosecond())); err != nil {
		return nil, fmt.Errorf("tau error: %s", err.Error())
	}
	return b.Bytes(), nil
}
