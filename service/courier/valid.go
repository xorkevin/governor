package courier

import (
	"github.com/hackform/governor"
	"net/http"
	"net/url"
	"regexp"
)

const (
	lengthCap     = 63
	lengthCapLink = 63
	lengthCapURL  = 2047
	amountCap     = 1024
)

var (
	linkRegex = regexp.MustCompile(`^[a-z][a-z0-9_-]+$`)
)

func validLinkID(linkid string) error {
	if len(linkid) == 0 {
		return nil
	}
	if len(linkid) > lengthCapLink {
		return governor.NewErrorUser("Link must be shorter than 64 characters", http.StatusBadRequest, nil)
	}
	if !linkRegex.MatchString(linkid) {
		return governor.NewErrorUser("Link can only contain a-z,0-9,_,-", http.StatusBadRequest, nil)
	}
	return nil
}

func validURL(rawurl string) error {
	if len(rawurl) < 1 {
		return governor.NewErrorUser("Url must be provided", http.StatusBadRequest, nil)
	}
	if len(rawurl) > lengthCapURL {
		return governor.NewErrorUser("Url is too long", http.StatusBadRequest, nil)
	}
	if _, err := url.ParseRequestURI(rawurl); err != nil {
		return governor.NewErrorUser("Url is invalid", http.StatusBadRequest, nil)
	}
	return nil
}

func hasCreatorID(creatorid string) error {
	if len(creatorid) < 1 || len(creatorid) > lengthCap {
		return governor.NewErrorUser("Creatorid must be provided", http.StatusBadRequest, nil)
	}
	return nil
}

func hasLinkID(linkid string) error {
	if len(linkid) < 3 || len(linkid) > lengthCapLink {
		return governor.NewErrorUser("Link must be longer than 2 chars", http.StatusBadRequest, nil)
	}
	return nil
}

func validAmount(amt int) error {
	if amt < 1 || amt > amountCap {
		return governor.NewErrorUser("Amount is invalid", http.StatusBadRequest, nil)
	}
	return nil
}

func validOffset(offset int) error {
	if offset < 0 {
		return governor.NewErrorUser("Offset is invalid", http.StatusBadRequest, nil)
	}
	return nil
}
