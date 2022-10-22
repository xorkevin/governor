package conduit

import (
	"net/http"
	"regexp"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/kjson"
)

//go:generate forge validation

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
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Chat id must be provided")
	}
	if len(chatid) > lengthCapChatid {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Chat id must be shorter than 32 characters")
	}
	return nil
}

func validhasChatids(chatids []string) error {
	if len(chatids) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "IDs must be provided")
	}
	if len(chatids) > amountCap {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Request is too large")
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
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Server id must be provided")
	}
	if len(serverid) > lengthCapServerID {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Server id must be shorter than 32 characters")
	}
	return nil
}

func validChannelID(channelid string) error {
	if len(channelid) < 3 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Channel id must be longer than 2 characters")
	}
	if len(channelid) > lengthCapChannelID {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Channel id must be shorter than 32 characters")
	}
	return nil
}

func validhasChannelID(channelid string) error {
	if len(channelid) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Channel id must be provided")
	}
	if len(channelid) > lengthCapChannelID {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Channel id must be shorter than 32 characters")
	}
	return nil
}

func validName(name string) error {
	if len(name) > lengthCapName {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Name must be shorter than 128 characters")
	}
	return nil
}

func validDesc(desc string) error {
	if len(desc) > lengthCapDesc {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Description must be shorter than 128 characters")
	}
	return nil
}

func validSearch(search string) error {
	if len(search) > lengthCapName {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Search must be shorter than 128 characters")
	}
	return nil
}

func validTheme(theme string) error {
	if len(theme) > lengthCapTheme {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Theme must be shorter than 4096 characters")
	}
	if err := kjson.Unmarshal([]byte(theme), &struct{}{}); err != nil {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Theme is invalid JSON")
	}
	return nil
}

func validhasUserid(userid string) error {
	if len(userid) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Userid must be provided")
	}
	if len(userid) > lengthCapUserid {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Userid must be shorter than 32 characters")
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

func validoptUserids(members []string) error {
	if len(members) > amountCap {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Request is too large")
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
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Username must be provided")
	}
	if len(username) > lengthCapName {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Username must be shorter than 128 characters")
	}
	return nil
}

func validoptUsername(username string) error {
	if len(username) > lengthCapName {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Username must be shorter than 128 characters")
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

func validhasMsgid(msgid string) error {
	if len(msgid) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Msgid must be provided")
	}
	if len(msgid) > lengthCapMsgid {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Msgid must be shorter than 32 characters")
	}
	return nil
}

func validoptMsgid(msgid string) error {
	if len(msgid) > lengthCapMsgid {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Msgid must be shorter than 32 characters")
	}
	return nil
}

func validMsgkind(kind string) error {
	if len(kind) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Msg kind must be provided")
	}
	switch kind {
	case chatMsgKindTxt:
	default:
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Invalid chat msg kind")
	}
	return nil
}

func validoptMsgkind(kind string) error {
	if len(kind) > lengthCapKind {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Msg kind must be shorter than 32 characters")
	}
	return nil
}

func validMsgvalue(value string) error {
	if len(value) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Msg value must be provided")
	}
	if len(value) > lengthCapMsg {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Msg value must be shorter than 4096 characters")
	}
	return nil
}
