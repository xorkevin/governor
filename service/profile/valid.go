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

func hasUserid(userid string) error {
	if len(userid) < 1 || len(userid) > lengthCap {
		return governor.NewErrorUser("Userid must be provided", http.StatusBadRequest, nil)
	}
	return nil
}

func validEmail(email string) error {
	if len(email) == 0 {
		return nil
	}
	if !emailRegex.MatchString(email) || len(email) > lengthCapEmail {
		return governor.NewErrorUser("Email is invalid", http.StatusBadRequest, nil)
	}
	return nil
}

func validBio(bio string) error {
	if len(bio) == 0 {
		return nil
	}
	if len(bio) > lengthCapLarge {
		return governor.NewErrorUser("Bio exceeds max length", http.StatusBadRequest, nil)
	}
	return nil
}
