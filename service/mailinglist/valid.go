package mailinglist

import (
	"net/http"

	"xorkevin.dev/governor"
)

const (
	lengthCapCreatorID = 31
	lengthCapUserid    = 31
	lengthCapListid    = 255
	lengthCapListname  = 127
	amountCap          = 255
)

func validhasCreatorID(creatorid string) error {
	if len(creatorid) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Creator id must be provided",
			Status:  http.StatusBadRequest,
		}))
	}
	if len(creatorid) > lengthCapCreatorID {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Creator id must be shorter than 32 characters",
			Status:  http.StatusBadRequest,
		}))
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

func validhasListid(listid string) error {
	if len(listid) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Listid must be provided",
			Status:  http.StatusBadRequest,
		}))
	}
	if len(listid) > lengthCapListid {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Listid must be shorter than 256 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validhasListname(listname string) error {
	if len(listname) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "List name must be provided",
			Status:  http.StatusBadRequest,
		}))
	}
	if len(listname) > lengthCapListname {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "List name must be shorter than 128 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validAmount(amt int) error {
	if amt < 1 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Amount must be positive",
			Status:  http.StatusBadRequest,
		}))
	}
	if amt > amountCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Amount must be less than 256",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}
