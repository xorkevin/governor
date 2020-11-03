package org

import (
	"net/http"
	"regexp"
	"xorkevin.dev/governor"
)

const (
	lengthCapUserid = 31
	lengthCapOrgID  = 31
	lengthCap       = 127
	lengthCapLarge  = 4095
	amountCap       = 1024
)

var (
	userRegex = regexp.MustCompile(`^[a-z][a-z0-9._-]+$`)
)

func validhasOrgid(orgid string) error {
	if len(orgid) == 0 {
		return governor.NewErrorUser("Org id must be provided", http.StatusBadRequest, nil)
	}
	if len(orgid) > lengthCapOrgID {
		return governor.NewErrorUser("Org id must be shorter than 32 characters", http.StatusBadRequest, nil)
	}
	return nil
}

func validName(name string) error {
	if len(name) < 3 {
		return governor.NewErrorUser("Org name must be longer than 2 characters", http.StatusBadRequest, nil)
	}
	if len(name) > lengthCap {
		return governor.NewErrorUser("Org name must be shorter than 128 characters", http.StatusBadRequest, nil)
	}
	if !userRegex.MatchString(name) {
		return governor.NewErrorUser("Org name contains invalid characters", http.StatusBadRequest, nil)
	}
	return nil
}

func validDisplay(display string) error {
	if len(display) == 0 {
		return governor.NewErrorUser("Org display name must be provided", http.StatusBadRequest, nil)
	}
	if len(display) > lengthCap {
		return governor.NewErrorUser("Org display name must be shorter than 128 characters", http.StatusBadRequest, nil)
	}
	return nil
}

func validDesc(desc string) error {
	if len(desc) > lengthCap {
		return governor.NewErrorUser("Org description must be shorter than 128 characters", http.StatusBadRequest, nil)
	}
	return nil
}

func validhasName(name string) error {
	if len(name) == 0 {
		return governor.NewErrorUser("Org name must be provided", http.StatusBadRequest, nil)
	}
	if len(name) > lengthCap {
		return governor.NewErrorUser("Org name must be shorter than 128 characters", http.StatusBadRequest, nil)
	}
	return nil
}

func validhasOrgids(orgids string) error {
	if len(orgids) == 0 {
		return governor.NewErrorUser("IDs must be provided", http.StatusBadRequest, nil)
	}
	if len(orgids) > lengthCapLarge {
		return governor.NewErrorUser("Request is too large", http.StatusBadRequest, nil)
	}
	return nil
}

func validhasUserid(userid string) error {
	if len(userid) == 0 {
		return governor.NewErrorUser("Userid must be provided", http.StatusBadRequest, nil)
	}
	if len(userid) > lengthCapUserid {
		return governor.NewErrorUser("Userid must be shorter than 32 characters", http.StatusBadRequest, nil)
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
