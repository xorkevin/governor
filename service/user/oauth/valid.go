package oauth

import (
	"net/http"
	"net/url"

	"xorkevin.dev/governor"
)

//go:generate forge validation

const (
	lengthCapClientID = 31
	lengthCapUserid   = 31
	lengthCapName     = 127
	amountCap         = 255
	lengthCapURL      = 512
	lengthCapRedirect = 512
)

func validhasUserid(userid string) error {
	if len(userid) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Userid must be provided")
	}
	if len(userid) > lengthCapUserid {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Userid must be shorter than 32 characters")
	}
	return nil
}

func validoptUserid(userid string) error {
	if len(userid) > lengthCapUserid {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Userid must be shorter than 32 characters")
	}
	return nil
}

func validhasClientID(clientid string) error {
	if len(clientid) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Client id must be provided")
	}
	if len(clientid) > lengthCapClientID {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Client id must be shorter than 32 characters")
	}
	return nil
}

func validhasClientIDs(clientids []string) error {
	if len(clientids) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "IDs must be provided")
	}
	if len(clientids) > amountCap {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Request is too large")
	}
	for _, i := range clientids {
		if err := validhasClientID(i); err != nil {
			return err
		}
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

func validName(name string) error {
	if len(name) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Name must be provided")
	}
	if len(name) > lengthCapName {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Name must be shorter than 128 characters")
	}
	return nil
}

func validURL(rawurl string) error {
	if len(rawurl) == 0 {
		return nil
	}
	if len(rawurl) > lengthCapURL {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "URL must be shorter than 513 characters")
	}
	if u, err := url.Parse(rawurl); err != nil || !u.IsAbs() || u.Hostname() == "" {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "URL is invalid")
	}
	return nil
}

func validRedirect(rawurl string) error {
	if len(rawurl) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Redirect URI must be provided")
	}
	if len(rawurl) > lengthCapRedirect {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Redirect URI must be shorter than 513 characters")
	}
	if u, err := url.Parse(rawurl); err != nil || !u.IsAbs() || u.Hostname() == "" || u.Fragment != "" {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Redirect URI is invalid")
	}
	return nil
}
