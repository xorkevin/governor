package profile

import (
	"github.com/hackform/governor"
	"io"
	"net/http"
	"regexp"
)

const (
	moduleIDReqValid = moduleID + ".reqvalid"
	lengthCap        = 128
	lengthCapLarge   = 4096
)

var (
	emailRegex    = regexp.MustCompile(`^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]+$`)
	urlImageRegex = regexp.MustCompile(`^https?://(?:[a-z0-9\-]+\.)+[a-z]{2,6}(?:/[^/#?]+)+\.(?:jpg|gif|png)$`)
)

func hasUserid(userid string) *governor.Error {
	if len(userid) < 1 || len(userid) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "userid must be provided", 0, http.StatusBadRequest)
	}
	return nil
}

func validEmail(email string) *governor.Error {
	if len(email) == 0 {
		return nil
	}
	if !emailRegex.MatchString(email) || len(email) > lengthCapLarge {
		return governor.NewErrorUser(moduleIDReqValid, "email is invalid", 0, http.StatusBadRequest)
	}
	return nil
}

func validBio(bio string) *governor.Error {
	if len(bio) == 0 {
		return nil
	}
	if len(bio) > lengthCapLarge {
		return governor.NewErrorUser(moduleIDReqValid, "bio exceeds max length", 0, http.StatusBadRequest)
	}
	return nil
}

func validImage(image io.Reader) *governor.Error {
	if image == nil {
		return governor.NewErrorUser(moduleIDReqValid, "image is invalid", 0, http.StatusBadRequest)
	}
	return nil
}

func validImageType(imagetype string) *governor.Error {
	switch imagetype {
	case "image/jpeg", "image/png", "image/gif":
		return nil
	default:
		return governor.NewErrorUser(moduleIDReqValid, "image type is invalid", 0, http.StatusBadRequest)
	}
}
