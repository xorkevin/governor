package post

import (
	"github.com/hackform/governor"
	"net/http"
	"regexp"
	"strings"
)

const (
	moduleIDReqValid = moduleID + ".reqvalid"
	lengthCap        = 128
	lengthCapLarge   = 4096
	lengthCapL2      = 65536
	lengthCapM2      = 1024
	amountCap        = 1024
)

var (
	groupRegex = regexp.MustCompile(`^[a-z][a-z0-9.-_]+$`)
)

func hasPostid(postid string) *governor.Error {
	if len(postid) < 1 || len(postid) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "postid must be provided", 0, http.StatusBadRequest)
	}
	return nil
}

func hasUserid(userid string) *governor.Error {
	if len(userid) < 1 || len(userid) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "userid must be provided", 0, http.StatusBadRequest)
	}
	return nil
}

func validTitle(title string) *governor.Error {
	if len(title) == 0 {
		return governor.NewErrorUser(moduleIDReqValid, "title must be provided", 0, http.StatusBadRequest)
	}
	if len(title) > lengthCapM2 {
		return governor.NewErrorUser(moduleIDReqValid, "title exceeds max length", 0, http.StatusBadRequest)
	}
	return nil
}

func validContent(content string) *governor.Error {
	if len(content) == 0 {
		return governor.NewErrorUser(moduleIDReqValid, "content must be provided", 0, http.StatusBadRequest)
	}
	if len(content) > lengthCapL2 {
		return governor.NewErrorUser(moduleIDReqValid, "content exceeds max length", 0, http.StatusBadRequest)
	}
	if strings.Contains(content, editEscapeSequence) {
		return governor.NewErrorUser(moduleIDReqValid, "content contains illegal characters", 0, http.StatusBadRequest)
	}
	return nil
}

func validGroup(groupTag string) *governor.Error {
	if len(groupTag) < 1 || len(groupTag) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "group tag must be provided", 0, http.StatusBadRequest)
	}
	if !groupRegex.MatchString(groupTag) {
		return governor.NewErrorUser(moduleIDReqValid, "group tag contains invalid characters", 0, http.StatusBadRequest)
	}
	return nil
}

func validAction(action string) *governor.Error {
	if len(action) < 1 || len(action) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "action must be provided", 0, http.StatusBadRequest)
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
