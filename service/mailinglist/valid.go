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
	lengthCapMsgid     = 1023
	lengthCapName      = 127
	amountCap          = 255
)

func validhasCreatorID(creatorid string) error {
	if len(creatorid) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Creator id must be provided")
	}
	if len(creatorid) > lengthCapCreatorID {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Creator id must be shorter than 32 characters")
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
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Userids must be provided")
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

func validhasListid(listid string) error {
	if len(listid) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Listid must be provided")
	}
	if len(listid) > lengthCapListid {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Listid must be shorter than 256 characters")
	}
	return nil
}

func validListname(listname string) error {
	if len(listname) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "List name must be provided")
	}
	if len(listname) > lengthCapListname {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "List name must be shorter than 128 characters")
	}
	return nil
}

func validhasListname(listname string) error {
	if len(listname) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "List name must be provided")
	}
	if len(listname) > lengthCapListname {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "List name must be shorter than 128 characters")
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

func validDesc(desc string) error {
	if len(desc) > lengthCapName {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Description must be shorter than 128 characters")
	}
	return nil
}

func validSenderPolicy(pol string) error {
	switch pol {
	case listSenderPolicyOwner, listSenderPolicyMember, listSenderPolicyUser:
		return nil
	default:
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Invalid sender policy")
	}
}

func validMemberPolicy(pol string) error {
	switch pol {
	case listMemberPolicyOwner, listMemberPolicyUser:
		return nil
	default:
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Invalid member policy")
	}
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
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Msg id must be provided")
	}
	if len(msgid) > lengthCapMsgid {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Msg id must be shorter than 1024 characters")
	}
	return nil
}

func validhasMsgids(msgids []string) error {
	if len(msgids) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Msg ids must be provided")
	}
	if len(msgids) > amountCap {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Request is too large")
	}
	for _, i := range msgids {
		if err := validhasMsgid(i); err != nil {
			return err
		}
	}
	return nil
}
