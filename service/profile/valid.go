package profile

import (
	"net/http"
	"net/mail"

	"xorkevin.dev/governor"
)

const (
	lengthCap      = 31
	lengthCapEmail = 254
	lengthCapLarge = 4095
	amountCap      = 255
)

func validhasUserid(userid string) error {
	if len(userid) < 1 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Userid must be provided",
		}))
	}
	if len(userid) > lengthCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Userid must be shorter than 32 characters",
		}))
	}
	return nil
}

func validEmail(email string) error {
	if len(email) == 0 {
		return nil
	}
	if len(email) > lengthCapEmail {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Email must be shorter than 255 characters",
		}))
	}
	a, err := mail.ParseAddress(email)
	if err != nil {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Email is invalid",
		}), governor.ErrOptInner(err))
	}
	if a.Address != email {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Email is invalid",
		}))
	}
	return nil
}

func validBio(bio string) error {
	if len(bio) > lengthCapLarge {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Bio must be shorter than 4096 characters",
		}))
	}
	return nil
}

func validhasUserids(userids []string) error {
	if len(userids) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "IDs must be provided",
		}))
	}
	if len(userids) > amountCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Request is too large",
		}))
	}
	for _, i := range userids {
		if err := validhasUserid(i); err != nil {
			return err
		}
	}
	return nil
}
