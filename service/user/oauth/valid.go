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
)

func validhasUserid(userid string) error {
	if len(userid) == 0 {
		return governor.NewErrorUser("Userid must be provided", http.StatusBadRequest, nil)
	}
	if len(userid) > lengthCapUserid {
		return governor.NewErrorUser("Userid must be shorter than 32 characters", http.StatusBadRequest, nil)
	}
	return nil
}

func validhasClientID(clientid string) error {
	if len(clientid) == 0 {
		return governor.NewErrorUser("Client id must be provided", http.StatusBadRequest, nil)
	}
	if len(clientid) > lengthCapClientID {
		return governor.NewErrorUser("Client id must be shorter than 32 characters", http.StatusBadRequest, nil)
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

func validName(name string) error {
	if len(name) == 0 {
		return governor.NewErrorUser("Name must be provided", http.StatusBadRequest, nil)
	}
	if len(name) > lengthCap {
		return governor.NewErrorUser("Name must be shorter than 128 characters", http.StatusBadRequest, nil)
	}
	return nil
}

func validURL(rawurl string) error {
	if len(rawurl) == 0 {
		return nil
	}
	if len(rawurl) > lengthCapURL {
		return governor.NewErrorUser("URL must be shorter than 513 characters", http.StatusBadRequest, nil)
	}
	if u, err := url.Parse(rawurl); err != nil || !u.IsAbs() {
		return governor.NewErrorUser("URL is invalid", http.StatusBadRequest, nil)
	}
	return nil
}

func validRedirect(rawurl string) error {
	if len(rawurl) == 0 {
		return governor.NewErrorUser("Redirect URI must be provided", http.StatusBadRequest, nil)
	}
	if len(rawurl) > lengthCapRedirect {
		return governor.NewErrorUser("Redirect URI must be shorter than 513 characters", http.StatusBadRequest, nil)
	}
	if u, err := url.Parse(rawurl); err != nil || !u.IsAbs() || u.Fragment != "" {
		return governor.NewErrorUser("Redirect URI is invalid", http.StatusBadRequest, nil)
	}
	return nil
}
