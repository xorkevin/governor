package org

import (
	"net/http"
	"xorkevin.dev/governor"
)

const (
	lengthCapOrgID = 31
	lengthCap      = 127
	lengthCapLarge = 4095
	amountCap      = 1024
)

func validhasOrgID(orgid string) error {
	if len(orgid) == 0 {
		return governor.NewErrorUser("Org id must be provided", http.StatusBadRequest, nil)
	}
	if len(orgid) > lengthCapOrgID {
		return governor.NewErrorUser("Org id must be shorter than 32 characters", http.StatusBadRequest, nil)
	}
	return nil
}

func validhasOrgName(name string) error {
	if len(name) == 0 {
		return governor.NewErrorUser("Org name must be provided", http.StatusBadRequest, nil)
	}
	if len(name) > lengthCap {
		return governor.NewErrorUser("Org name must be shorter than 128 characters", http.StatusBadRequest, nil)
	}
	return nil
}

func validhasOrgIDs(orgids string) error {
	if len(orgids) == 0 {
		return governor.NewErrorUser("IDs must be provided", http.StatusBadRequest, nil)
	}
	if len(orgids) > lengthCapLarge {
		return governor.NewErrorUser("Request is too large", http.StatusBadRequest, nil)
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
