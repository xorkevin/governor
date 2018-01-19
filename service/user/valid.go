package user

import (
	"fmt"
	"github.com/hackform/governor"
	"github.com/hackform/governor/util/rank"
	"net/http"
	"regexp"
)

const (
	moduleIDReqValid = moduleID + ".reqvalid"
	lengthCap        = 128
	lengthCapLarge   = 4096
	amountCap        = 1024
)

var (
	userRegex  = regexp.MustCompile(`^[a-z][a-z0-9.-_]+$`)
	emailRegex = regexp.MustCompile(`^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]+$`)
)

func validUsername(username string) *governor.Error {
	if len(username) < 3 || len(username) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "username must be longer than 2 chars", 0, http.StatusBadRequest)
	}
	if !userRegex.MatchString(username) {
		return governor.NewErrorUser(moduleIDReqValid, "username contains invalid characters", 0, http.StatusBadRequest)
	}
	return nil
}

func validPassword(password string, size int) *governor.Error {
	if len(password) < size || len(password) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, fmt.Sprintf("password must be longer than %d chars", size), 0, http.StatusBadRequest)
	}
	return nil
}

func validEmail(email string) *governor.Error {
	if !emailRegex.MatchString(email) || len(email) > lengthCapLarge {
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
	if _, err := rank.FromStringUser(rankString); err != nil {
		return err
	}
	return nil
}

func hasUserid(userid string) *governor.Error {
	if len(userid) < 1 || len(userid) > lengthCapLarge {
		return governor.NewErrorUser(moduleIDReqValid, "userid must be provided", 0, http.StatusBadRequest)
	}
	return nil
}

func hasUsername(username string) *governor.Error {
	if len(username) < 1 || len(username) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "username must be provided", 0, http.StatusBadRequest)
	}
	if !userRegex.MatchString(username) {
		return governor.NewErrorUser(moduleIDReqValid, "username contains invalid characters", 0, http.StatusBadRequest)
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
	if len(token) < 1 || len(token) > lengthCapLarge {
		return governor.NewErrorUser(moduleIDReqValid, "token must be provided", 0, http.StatusBadRequest)
	}
	return nil
}

func hasIDs(ids []string) *governor.Error {
	if len(ids) < 1 {
		return governor.NewErrorUser(moduleIDReqValid, "ids must be provided", 0, http.StatusBadRequest)
	}
	return nil
}

func validAmount(amt int) *governor.Error {
	if amt < 1 || amt > amountCap {
		return governor.NewErrorUser(moduleIDReqValid, "amount is invalid", 0, http.StatusBadRequest)
	}
	return nil
}

func validOffset(offset int) *governor.Error {
	if offset < 0 {
		return governor.NewErrorUser(moduleIDReqValid, "offset is invalid", 0, http.StatusBadRequest)
	}
	return nil
}

func validRole(role string) *governor.Error {
	if len(role) < 0 || len(role) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "role is invalid", 0, http.StatusBadRequest)
	}
	return nil
}
