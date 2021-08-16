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
	amountCap       = 255
)

var (
	userRegex = regexp.MustCompile(`^[a-z][a-z0-9._-]+$`)
)

func validhasOrgid(orgid string) error {
	if len(orgid) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Org id must be provided",
			Status:  http.StatusBadRequest,
		}))
	}
	if len(orgid) > lengthCapOrgID {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Org id must be shorter than 32 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validName(name string) error {
	if len(name) < 3 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Org name must be longer than 2 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	if len(name) > lengthCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Org name must be shorter than 128 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	if !userRegex.MatchString(name) {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Org name contains invalid characters",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validDisplay(display string) error {
	if len(display) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Org display name must be provided",
			Status:  http.StatusBadRequest,
		}))
	}
	if len(display) > lengthCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Org display name must be shorter than 128 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validDesc(desc string) error {
	if len(desc) > lengthCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Org description must be shorter than 128 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validhasName(name string) error {
	if len(name) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Org name must be provided",
			Status:  http.StatusBadRequest,
		}))
	}
	if len(name) > lengthCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Org name must be shorter than 128 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validhasOrgids(orgids []string) error {
	if len(orgids) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "IDs must be provided",
		}))
	}
	if len(orgids) > amountCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Request is too large",
		}))
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
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Userid must be provided",
			Status:  http.StatusBadRequest,
		}))
	}
	if len(userid) > lengthCapUserid {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Userid must be shorter than 32 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validAmount(amt int) error {
	if amt == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Amount must be positive",
			Status:  http.StatusBadRequest,
		}))
	}
	if amt > amountCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Amount must be less than 1024",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validOffset(offset int) error {
	if offset < 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Offset must not be negative",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}
