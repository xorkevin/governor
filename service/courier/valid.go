package courier

import (
	"net/http"
	"net/url"
	"regexp"
	"xorkevin.dev/governor"
)

const (
	lengthCap     = 31
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
	if len(linkid) < 3 {
		return governor.NewErrorUser("Link id must be longer than 2 characters", http.StatusBadRequest, nil)
	}
	if len(linkid) > lengthCapLink {
		return governor.NewErrorUser("Link id must be shorter than 64 characters", http.StatusBadRequest, nil)
	}
	if !linkRegex.MatchString(linkid) {
		return governor.NewErrorUser("Link id can only contain a-z,0-9,_,-", http.StatusBadRequest, nil)
	}
	return nil
}

func validhasLinkID(linkid string) error {
	if len(linkid) == 0 {
		return governor.NewErrorUser("Link id must be provided", http.StatusBadRequest, nil)
	}
	if len(linkid) > lengthCapLink {
		return governor.NewErrorUser("Link id must be shorter than 64 characters", http.StatusBadRequest, nil)
	}
	return nil
}

func validURL(rawurl string) error {
	if len(rawurl) == 0 {
		return governor.NewErrorUser("Url must be provided", http.StatusBadRequest, nil)
	}
	if len(rawurl) > lengthCapURL {
		return governor.NewErrorUser("Url must be shorter than 2048 characters", http.StatusBadRequest, nil)
	}
	if _, err := url.ParseRequestURI(rawurl); err != nil {
		return governor.NewErrorUser("Url is invalid", http.StatusBadRequest, nil)
	}
	return nil
}

func validhasCreatorID(creatorid string) error {
	if len(creatorid) == 0 {
		return governor.NewErrorUser("Creatorid must be provided", http.StatusBadRequest, nil)
	}
	if len(creatorid) > lengthCap {
		return governor.NewErrorUser("Creatorid must be shorter than 32 characters", http.StatusBadRequest, nil)
	}
	return nil
}

func validAmount(amt int) error {
	if amt == 0 {
		return governor.NewErrorUser("Amount must be positive", http.StatusBadRequest, nil)
	}
	if amt > amountCap {
		return governor.NewErrorUser("Amount must be less than 1024", http.StatusBadRequest, nil)
	}
	return nil
}

func validOffset(offset int) error {
	if offset < 0 {
		return governor.NewErrorUser("Offset must not be negative", http.StatusBadRequest, nil)
	}
	return nil
}
