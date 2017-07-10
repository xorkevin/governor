package profile

import (
	"github.com/hackform/governor"
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

func validImage(imageurl string) *governor.Error {
	if len(imageurl) == 0 {
		return nil
	}
	if !urlImageRegex.MatchString(imageurl) || len(imageurl) > lengthCapLarge {
		return governor.NewErrorUser(moduleIDReqValid, "profile image url is invalid", 0, http.StatusBadRequest)
	}
	return nil
}

func validSetPublic(setpublic string) *governor.Error {
	if len(setpublic) > lengthCapLarge {
		return governor.NewErrorUser(moduleIDReqValid, "set of public fields is invalid", 0, http.StatusBadRequest)
	}
	return nil
}