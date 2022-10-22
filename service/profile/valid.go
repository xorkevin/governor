package profile

import (
	"net/http"
	"net/mail"

	"xorkevin.dev/governor"
)

//go:generate forge validation

const (
	lengthCap      = 31
	lengthCapEmail = 254
	lengthCapLarge = 4095
	amountCap      = 255
)

func validhasUserid(userid string) error {
	if len(userid) < 1 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Userid must be provided")
	}
	if len(userid) > lengthCap {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Userid must be shorter than 32 characters")
	}
	return nil
}

func validEmail(email string) error {
	if len(email) == 0 {
		return nil
	}
	if len(email) > lengthCapEmail {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Email must be shorter than 255 characters")
	}
	a, err := mail.ParseAddress(email)
	if err != nil {
		return governor.ErrWithRes(err, http.StatusBadRequest, "", "Email is invalid")
	}
	if a.Address != email {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Email is invalid")
	}
	return nil
}

func validBio(bio string) error {
	if len(bio) > lengthCapLarge {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Bio must be shorter than 4096 characters")
	}
	return nil
}

func validhasUserids(userids []string) error {
	if len(userids) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "IDs must be provided")
	}
	if len(userids) > amountCap {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Request is too large")
	}
	for _, i := range userids {
		if err := validhasUserid(i); err != nil {
			return err
		}
	}
	return nil
}
