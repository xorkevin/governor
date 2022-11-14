package courier

import (
	"net/http"
	"net/url"
	"regexp"

	"xorkevin.dev/governor"
)

//go:generate forge validation

const (
	lengthCapCreatorID = 31
	lengthCapID        = 63
	lengthCapURL       = 2047
	amountCap          = 255
)

var (
	idRegex = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

func validLinkID(linkid string) error {
	if len(linkid) == 0 {
		return nil
	}
	if len(linkid) < 3 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Link id must be longer than 2 characters")
	}
	if len(linkid) > lengthCapID {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Link id must be shorter than 64 characters")
	}
	if !idRegex.MatchString(linkid) {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Link id can only contain A-Z,a-z,0-9,_,-")
	}
	return nil
}

func validhasLinkID(linkid string) error {
	if len(linkid) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Link id must be provided")
	}
	if len(linkid) > lengthCapID {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Link id must be shorter than 64 characters")
	}
	return nil
}

func validBrandID(brandid string) error {
	if len(brandid) < 3 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Brand id must be longer than 2 characters")
	}
	if len(brandid) > lengthCapID {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Brand id must be shorter than 64 characters")
	}
	if !idRegex.MatchString(brandid) {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Brand id can only contain a-z,0-9,_,-")
	}
	return nil
}

func validhasBrandID(brandid string) error {
	if len(brandid) == 0 {
		return nil
	}
	if len(brandid) > lengthCapID {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Brand id must be shorter than 64 characters")
	}
	return nil
}

func validURL(rawurl string) error {
	if len(rawurl) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Url must be provided")
	}
	if len(rawurl) > lengthCapURL {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Url must be shorter than 2048 characters")
	}
	if u, err := url.Parse(rawurl); err != nil || !u.IsAbs() || u.Hostname() == "" {
		return governor.ErrWithRes(err, http.StatusBadRequest, "", "Url is invalid")
	}
	return nil
}

func validhasCreatorID(creatorid string) error {
	if len(creatorid) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Creator id must be provided")
	}
	if len(creatorid) > lengthCapCreatorID {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Creator id must be shorter than 32 characters")
	}
	return nil
}

func validAmount(amt int) error {
	if amt < 1 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Amount must be positive")
	}
	if amt > amountCap {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Amount must be less than 256")
	}
	return nil
}

func validOffset(offset int) error {
	if offset < 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Offset must not be negative")
	}
	return nil
}
