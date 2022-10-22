package org

import (
	"net/http"
	"regexp"

	"xorkevin.dev/governor"
)

//go:generate forge validation

const (
	lengthCapUserid = 31
	lengthCapOrgID  = 31
	lengthCapName   = 127
	amountCap       = 255
)

var (
	orgRegex = regexp.MustCompile(`^[a-z0-9_-]+$`)
)

func validhasOrgid(orgid string) error {
	if len(orgid) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Org id must be provided")
	}
	if len(orgid) > lengthCapOrgID {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Org id must be shorter than 32 characters")
	}
	return nil
}

func validName(name string) error {
	if len(name) < 3 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Org name must be longer than 2 characters")
	}
	if len(name) > lengthCapName {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Org name must be shorter than 128 characters")
	}
	if !orgRegex.MatchString(name) {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Org name contains invalid characters")
	}
	return nil
}

func validDisplay(display string) error {
	if len(display) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Org display name must be provided")
	}
	if len(display) > lengthCapName {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Org display name must be shorter than 128 characters")
	}
	return nil
}

func validDesc(desc string) error {
	if len(desc) > lengthCapName {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Org description must be shorter than 128 characters")
	}
	return nil
}

func validhasName(name string) error {
	if len(name) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Org name must be provided")
	}
	if len(name) > lengthCapName {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Org name must be shorter than 128 characters")
	}
	return nil
}

func validoptName(name string) error {
	if len(name) > lengthCapName {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Org name must be shorter than 128 characters")
	}
	return nil
}

func validhasOrgids(orgids []string) error {
	if len(orgids) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "IDs must be provided")
	}
	if len(orgids) > amountCap {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Request is too large")
	}
	for _, i := range orgids {
		if err := validhasOrgid(i); err != nil {
			return err
		}
	}
	return nil
}

func validhasUserid(userid string) error {
	if len(userid) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Userid must be provided")
	}
	if len(userid) > lengthCapUserid {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Userid must be shorter than 32 characters")
	}
	return nil
}

func validoptUsername(username string) error {
	if len(username) > lengthCapName {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Username must be shorter than 128 characters")
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
