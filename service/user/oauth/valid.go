package oauth

import (
	"net/http"
	"net/url"

	"xorkevin.dev/governor"
)

const (
	lengthCapClientID = 31
	lengthCapUserid   = 31
	lengthCap         = 127
	amountCap         = 1024
	lengthCapURL      = 512
	lengthCapRedirect = 512
	lengthCapLarge    = 4095
)

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

func validoptUserid(userid string) error {
	if len(userid) > lengthCapUserid {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Userid must be shorter than 32 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validhasClientID(clientid string) error {
	if len(clientid) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Client id must be provided",
			Status:  http.StatusBadRequest,
		}))
	}
	if len(clientid) > lengthCapClientID {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Client id must be shorter than 32 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validhasClientIDs(clientids string) error {
	if len(clientids) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "IDs must be provided",
			Status:  http.StatusBadRequest,
		}))
	}
	if len(clientids) > lengthCapLarge {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Request is too large",
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

func validName(name string) error {
	if len(name) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Name must be provided",
			Status:  http.StatusBadRequest,
		}))
	}
	if len(name) > lengthCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Name must be shorter than 128 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validURL(rawurl string) error {
	if len(rawurl) == 0 {
		return nil
	}
	if len(rawurl) > lengthCapURL {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "URL must be shorter than 513 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	if u, err := url.Parse(rawurl); err != nil || !u.IsAbs() {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "URL is invalid",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validRedirect(rawurl string) error {
	if len(rawurl) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Redirect URI must be provided",
			Status:  http.StatusBadRequest,
		}))
	}
	if len(rawurl) > lengthCapRedirect {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Redirect URI must be shorter than 513 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	if u, err := url.Parse(rawurl); err != nil || !u.IsAbs() || u.Fragment != "" {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Redirect URI is invalid",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}
