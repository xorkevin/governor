// Code generated by go generate forge validation v0.4.0; DO NOT EDIT.

package mailinglist

func (r reqCreatorLists) valid() error {
	if err := validhasCreatorID(r.CreatorID); err != nil {
		return err
	}
	if err := validAmount(r.Amount); err != nil {
		return err
	}
	if err := validOffset(r.Offset); err != nil {
		return err
	}
	return nil
}

func (r reqUserLists) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validAmount(r.Amount); err != nil {
		return err
	}
	if err := validOffset(r.Offset); err != nil {
		return err
	}
	return nil
}

func (r reqList) valid() error {
	if err := validhasListid(r.Listid); err != nil {
		return err
	}
	return nil
}

func (r reqListMsgs) valid() error {
	if err := validhasListid(r.Listid); err != nil {
		return err
	}
	if err := validAmount(r.Amount); err != nil {
		return err
	}
	if err := validOffset(r.Offset); err != nil {
		return err
	}
	return nil
}

func (r reqListThread) valid() error {
	if err := validhasListid(r.Listid); err != nil {
		return err
	}
	if err := validhasMsgid(r.Threadid); err != nil {
		return err
	}
	if err := validAmount(r.Amount); err != nil {
		return err
	}
	if err := validOffset(r.Offset); err != nil {
		return err
	}
	return nil
}

func (r reqListMsg) valid() error {
	if err := validhasListid(r.Listid); err != nil {
		return err
	}
	if err := validhasMsgid(r.Msgid); err != nil {
		return err
	}
	return nil
}

func (r reqListMembers) valid() error {
	if err := validhasListid(r.Listid); err != nil {
		return err
	}
	if err := validhasUserids(r.Userids); err != nil {
		return err
	}
	return nil
}

func (r reqCreateList) valid() error {
	if err := validhasCreatorID(r.CreatorID); err != nil {
		return err
	}
	if err := validListname(r.Listname); err != nil {
		return err
	}
	if err := validName(r.Name); err != nil {
		return err
	}
	if err := validDesc(r.Desc); err != nil {
		return err
	}
	if err := validSenderPolicy(r.SenderPolicy); err != nil {
		return err
	}
	if err := validMemberPolicy(r.MemberPolicy); err != nil {
		return err
	}
	return nil
}

func (r reqUpdateList) valid() error {
	if err := validhasCreatorID(r.CreatorID); err != nil {
		return err
	}
	if err := validhasListname(r.Listname); err != nil {
		return err
	}
	if err := validName(r.Name); err != nil {
		return err
	}
	if err := validDesc(r.Desc); err != nil {
		return err
	}
	if err := validSenderPolicy(r.SenderPolicy); err != nil {
		return err
	}
	if err := validMemberPolicy(r.MemberPolicy); err != nil {
		return err
	}
	return nil
}

func (r reqSub) valid() error {
	if err := validhasCreatorID(r.CreatorID); err != nil {
		return err
	}
	if err := validhasListname(r.Listname); err != nil {
		return err
	}
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	return nil
}

func (r reqUpdListMembers) valid() error {
	if err := validhasCreatorID(r.CreatorID); err != nil {
		return err
	}
	if err := validhasListname(r.Listname); err != nil {
		return err
	}
	if err := validhasUserids(r.Remove); err != nil {
		return err
	}
	return nil
}

func (r reqListID) valid() error {
	if err := validhasCreatorID(r.CreatorID); err != nil {
		return err
	}
	if err := validhasListname(r.Listname); err != nil {
		return err
	}
	return nil
}

func (r reqMsgIDs) valid() error {
	if err := validhasCreatorID(r.CreatorID); err != nil {
		return err
	}
	if err := validhasListname(r.Listname); err != nil {
		return err
	}
	if err := validhasMsgids(r.Msgids); err != nil {
		return err
	}
	return nil
}