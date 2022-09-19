// Code generated by go generate forge validation v0.3; DO NOT EDIT.

package conduit

func (r reqGetFriends) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validoptUsername(r.Prefix); err != nil {
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

func (r reqSearchFriends) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validhasUsername(r.Prefix); err != nil {
		return err
	}
	if err := validAmount(r.Amount); err != nil {
		return err
	}
	return nil
}

func (r reqRmFriend) valid() error {
	if err := validhasUserid(r.Userid1); err != nil {
		return err
	}
	if err := validhasUserid(r.Userid2); err != nil {
		return err
	}
	return nil
}

func (r reqFriendInvitation) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validhasUserid(r.InvitedBy); err != nil {
		return err
	}
	return nil
}

func (r reqGetFriendInvitations) valid() error {
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