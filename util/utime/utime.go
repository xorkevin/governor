package utime

import (
	"bytes"
	"encoding/binary"
	"github.com/hackform/governor"
	"net/http"
	"time"
)

//////////
// Time //
//////////

const (
	moduleID          = "utime"
	moduleIDTimestamp = moduleID + ".Timestamp"
)

// Timestamp creates a new 12 byte array from Unixtime
// first 8 bytes are seconds and next 4 bytes are nanoseconds
func Timestamp() ([]byte, *governor.Error) {
	b := bytes.Buffer{}
	t := time.Now()
	if err := binary.Write(&b, binary.BigEndian, t.Unix()); err != nil {
		return nil, governor.NewError(moduleIDTimestamp, err.Error(), 0, http.StatusInternalServerError)
	}
	if err := binary.Write(&b, binary.BigEndian, int32(t.Nanosecond())); err != nil {
		return nil, governor.NewError(moduleIDTimestamp, err.Error(), 0, http.StatusInternalServerError)
	}
	return b.Bytes(), nil
}
