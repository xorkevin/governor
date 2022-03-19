package conduit

import (
	"encoding/json"
	"net/http"
	"regexp"

	"xorkevin.dev/governor"
)

const (
	lengthCapChatid    = 31
	lengthCapServerID  = 31
	lengthCapChannelID = 31
	lengthCapKind      = 31
	lengthCapName      = 127
	lengthCapDesc      = 127
	lengthCapTheme     = 4095
	lengthCapUserid    = 31
	lengthCapMsgid     = 31
	lengthCapMsg       = 4095
	amountCap          = 255
)

var (
	channelRegex = regexp.MustCompile(`^[a-z0-9_-]+$`)
)

func validhasChatid(chatid string) error {
	if len(chatid) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Chat id must be provided",
		}))
	}
	if len(chatid) > lengthCapChatid {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Chat id must be shorter than 32 characters",
		}))
	}
	return nil
}

func validhasChatids(chatids []string) error {
	if len(chatids) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "IDs must be provided",
		}))
	}
	if len(chatids) > amountCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Request is too large",
		}))
	}
	for _, i := range chatids {
		if err := validhasChatid(i); err != nil {
			return err
		}
	}
	return nil
}

func validhasServerID(serverid string) error {
	if len(serverid) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Server id must be provided",
			Status:  http.StatusBadRequest,
		}))
	}
	if len(serverid) > lengthCapServerID {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Server id must be shorter than 32 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validChannelID(channelid string) error {
	if len(channelid) < 3 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Channel id must be longer than 2 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	if len(channelid) > lengthCapChannelID {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Channel id must be shorter than 32 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validName(name string) error {
	if len(name) > lengthCapName {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Name must be shorter than 128 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validDesc(desc string) error {
	if len(desc) > lengthCapDesc {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Description must be shorter than 128 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validSearch(search string) error {
	if len(search) > lengthCapName {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Search must be shorter than 128 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validTheme(theme string) error {
	if len(theme) > lengthCapTheme {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Theme must be shorter than 4096 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	if err := json.Unmarshal([]byte(theme), &struct{}{}); err != nil {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Theme is invalid JSON",
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

func validhasUsername(username string) error {
	if len(username) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Username must be provided",
		}))
	}
	if len(username) > lengthCapName {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Username must be shorter than 128 characters",
		}))
	}
	return nil
}

func validoptUsername(username string) error {
	if len(username) > lengthCapName {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Username must be shorter than 128 characters",
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

func validOffset(offset int) error {
	if offset < 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Offset must not be negative",
		}))
	}
	return nil
}

func validhasMsgid(msgid string) error {
	if len(msgid) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Msgid must be provided",
			Status:  http.StatusBadRequest,
		}))
	}
	if len(msgid) > lengthCapMsgid {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Msgid must be shorter than 32 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validoptMsgid(msgid string) error {
	if len(msgid) > lengthCapMsgid {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Msgid must be shorter than 32 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validMsgkind(kind string) error {
	if len(kind) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Msg kind must be provided",
			Status:  http.StatusBadRequest,
		}))
	}
	switch kind {
	case chatMsgKindTxt:
	default:
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Invalid chat msg kind",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validoptMsgkind(kind string) error {
	if len(kind) > lengthCapKind {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Msg kind must be shorter than 32 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validMsgvalue(value string) error {
	if len(value) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Msg value must be provided",
			Status:  http.StatusBadRequest,
		}))
	}
	if len(value) > lengthCapMsg {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Msg value must be shorter than 4096 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}
