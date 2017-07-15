package post

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

func validContent(content string) *governor.Error {
	if len(content) == 0 {
		return governor.NewErrorUser(moduleIDReqValid, "content must be provided", 0, http.StatusBadRequest)
	}
	if len(content) > lengthCapLarge {
		return governor.NewErrorUser(moduleIDReqValid, "content exceeds max length", 0, http.StatusBadRequest)
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
