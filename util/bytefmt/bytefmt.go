package bytefmt

import (
	"errors"
	"strconv"
	"strings"
	"unicode"
)

var ErrFmt = errors.New("Invalid byte format")

const (
	BYTE = 1 << (10 * iota)
	KILOBYTE
	MEGABYTE
	GIGABYTE
	TERABYTE
	PETABYTE
)

func ToBytes(s string) (int64, error) {
	s = strings.ToUpper(s)

	i := strings.IndexFunc(s, unicode.IsLetter)

	if i < 0 {
		return 0, ErrFmt
	}

	bytesString, multiple := s[:i], s[i:]
	bytes, err := strconv.ParseInt(bytesString, 10, 64)
	if err != nil || bytes < 0 {
		return 0, ErrFmt
	}

	switch multiple {
	case "P", "PB", "PIB":
		return bytes * PETABYTE, nil
	case "T", "TB", "TIB":
		return bytes * TERABYTE, nil
	case "G", "GB", "GIB":
		return bytes * GIGABYTE, nil
	case "M", "MB", "MIB":
		return bytes * MEGABYTE, nil
	case "K", "KB", "KIB":
		return bytes * KILOBYTE, nil
	case "B":
		return bytes, nil
	default:
		return 0, ErrFmt
	}
}
