// Code generated by go generate forge validation v0.3; DO NOT EDIT.

package user

func (r reqGetUserSessions) valid() error {
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

func (r reqUserRmSessions) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validSessionIDs(r.SessionIDs); err != nil {
		return err
	}
	return nil
}
