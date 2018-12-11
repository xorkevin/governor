package courier

import (
	"github.com/hackform/governor"
	"net/http"
	"net/url"
	"regexp"
)

const (
	moduleIDReqValid = moduleID + ".reqvalid"
	lengthCap        = 63
	lengthCapLink    = 63
	lengthCapURL     = 2047
	amountCap        = 1024
)

var (
	linkRegex = regexp.MustCompile(`^[a-z][a-z0-9_-]+$`)
)

func validLinkID(linkid string) *governor.Error {
	if len(linkid) == 0 {
		return nil
	}
	if len(linkid) > lengthCapLink {
		return governor.NewErrorUser(moduleIDReqValid, "link must be shorter than 64 characters", 0, http.StatusBadRequest)
	}
	if !linkRegex.MatchString(linkid) {
		return governor.NewErrorUser(moduleIDReqValid, "link can only contain a-z,0-9,_,-", 0, http.StatusBadRequest)
	}
	return nil
}

func validURL(rawurl string) *governor.Error {
	if len(rawurl) < 1 {
		return governor.NewErrorUser(moduleIDReqValid, "url must be provided", 0, http.StatusBadRequest)
	}
	if len(rawurl) > lengthCapURL {
		return governor.NewErrorUser(moduleIDReqValid, "url is too long", 0, http.StatusBadRequest)
	}
	if _, err := url.ParseRequestURI(rawurl); err != nil {
		return governor.NewErrorUser(moduleIDReqValid, "url is invalid", 0, http.StatusBadRequest)
	}
	return nil
}

func hasCreatorID(creatorid string) *governor.Error {
	if len(creatorid) < 1 || len(creatorid) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "creatorid must be provided", 0, http.StatusBadRequest)
	}
	return nil
}

func hasLinkID(linkid string) *governor.Error {
	if len(linkid) < 3 || len(linkid) > lengthCapLink {
		return governor.NewErrorUser(moduleIDReqValid, "link must be longer than 2 chars", 0, http.StatusBadRequest)
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
