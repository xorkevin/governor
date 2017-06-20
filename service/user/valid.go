package user

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/util/rank"
	"net/http"
	"regexp"
)

const (
	moduleIDReqValid = moduleID + ".reqvalid"
	lengthCap        = 128
	lengthCapLarge   = 4096
)

var (
	emailRegex = regexp.MustCompile(`^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]+$`)
)

func validUsername(username string) *governor.Error {
	if len(username) < 3 || len(username) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "username must be longer than 2 chars", 0, http.StatusBadRequest)
	}
	return nil
}

func validPassword(password string) *governor.Error {
	if len(password) < 10 || len(password) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "password must be longer than 9 chars", 0, http.StatusBadRequest)
	}
	return nil
}

func validEmail(email string) *governor.Error {
	if !emailRegex.MatchString(email) || len(email) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "email is invalid", 0, http.StatusBadRequest)
	}
	return nil
}

func validFirstName(firstname string) *governor.Error {
	if len(firstname) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "first name is too long", 0, http.StatusBadRequest)
	}
	return nil
}

func validLastName(lastname string) *governor.Error {
	if len(lastname) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "last name is too long", 0, http.StatusBadRequest)
	}
	return nil
}

func validRank(rankString string) *governor.Error {
	if len(rankString) > lengthCapLarge {
		return governor.NewErrorUser(moduleIDReqValid, "rank exceeds the max length", 0, http.StatusBadRequest)
	}
	if _, err := rank.FromString(rankString); err != nil {
		return err
	}
	return nil
}

func hasUserid(userid string) *governor.Error {
	if len(userid) < 1 || len(userid) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "userid must be provided", 0, http.StatusBadRequest)
	}
	return nil
}

func hasUsername(username string) *governor.Error {
	if len(username) < 1 || len(username) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "username must be provided", 0, http.StatusBadRequest)
	}
	return nil
}

func hasPassword(password string) *governor.Error {
	if len(password) < 1 || len(password) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "password must be provided", 0, http.StatusBadRequest)
	}
	return nil
}

func hasToken(token string) *governor.Error {
	if len(token) < 1 || len(token) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "password must be provided", 0, http.StatusBadRequest)
	}
	return nil
}
