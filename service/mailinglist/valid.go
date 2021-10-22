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
	lengthCap          = 127
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

func validListname(listname string) error {
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

func validDesc(desc string) error {
	if len(desc) > lengthCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Description must be shorter than 128 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validSenderPolicy(pol string) error {
	switch pol {
	case listSenderPolicyOwner, listSenderPolicyMember, listSenderPolicyUser:
		return nil
	default:
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Invalid sender policy",
			Status:  http.StatusBadRequest,
		}))
	}
}

func validMemberPolicy(pol string) error {
	switch pol {
	case listMemberPolicyOwner, listMemberPolicyUser:
		return nil
	default:
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Invalid member policy",
			Status:  http.StatusBadRequest,
		}))
	}
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
			Message: "Msg id must be provided",
			Status:  http.StatusBadRequest,
		}))
	}
	if len(msgid) > lengthCapMsgid {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Msg id must be shorter than 1024 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validhasMsgids(msgids []string) error {
	if len(msgids) > amountCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Request is too large",
		}))
	}
	for _, i := range msgids {
		if err := validhasUserid(i); err != nil {
			return err
		}
	}
	return nil
}
