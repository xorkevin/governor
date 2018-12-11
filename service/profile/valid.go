package profile

import (
	"github.com/hackform/governor"
	"net/http"
	"regexp"
)

const (
	moduleIDReqValid = moduleID + ".reqvalid"
	lengthCap        = 128
	lengthCapEmail   = 255
	lengthCapLarge   = 4095
)

var (
	emailRegex = regexp.MustCompile(`^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]+$`)
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
	if !emailRegex.MatchString(email) || len(email) > lengthCapEmail {
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

func validKey(key string) *governor.Error {
	if len(key) < 1 || len(key) > lengthCapLarge {
		return governor.NewErrorUser(moduleIDReqValid, "key must be provided", 0, http.StatusBadRequest)
	}
	return nil
}
