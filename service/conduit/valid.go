package conduit

import (
	"encoding/json"
	"net/http"

	"xorkevin.dev/governor"
)

const (
	lengthCapChatid = 31
	lengthCapKind   = 31
	lengthCapName   = 127
	lengthCapTheme  = 4095
	lengthCapUserid = 31
	amountCap       = 255
)

func validhasChatid(chatid string) error {
	if len(chatid) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Chat id must be provided",
		}))
	}
	if len(chatid) > lengthCapUserid {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Chat id must be shorter than 32 characters",
		}))
	}
	return nil
}

func validKind(kind string) error {
	if len(kind) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Chat kind must be provided",
			Status:  http.StatusBadRequest,
		}))
	}
	if len(kind) > lengthCapKind {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Chat kind must be shorter than 32 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validName(name string) error {
	if len(name) > lengthCapName {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Chat name must be shorter than 256 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validTheme(theme string) error {
	if len(theme) > lengthCapTheme {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Chat theme must be shorter than 4096 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	if err := json.Unmarshal([]byte(theme), &struct{}{}); err != nil {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Chat theme is invalid JSON",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validhasUserid(userid string) error {
	if len(userid) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Userid must be provided",
		}))
	}
	if len(userid) > lengthCapUserid {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Userid must be shorter than 32 characters",
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

func validoptUserids(members []string) error {
	if len(members) > amountCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Request is too large",
		}))
	}
	for _, i := range members {
		if err := validhasUserid(i); err != nil {
			return err
		}
	}
	return nil
}
