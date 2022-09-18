// Code generated by go generate forge validation v0.3; DO NOT EDIT.

package user

func (r reqGetUserApikeys) valid() error {
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

func (r reqApikeyPost) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validScope(r.Scope); err != nil {
		return err
	}
	if err := validApikeyName(r.Name); err != nil {
		return err
	}
	if err := validApikeyDesc(r.Desc); err != nil {
		return err
	}
	return nil
}

func (r reqApikeyID) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validhasApikeyid(r.Keyid); err != nil {
		return err
	}
	return nil
}

func (r reqApikeyUpdate) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validhasApikeyid(r.Keyid); err != nil {
		return err
	}
	if err := validScope(r.Scope); err != nil {
		return err
	}
	if err := validApikeyName(r.Name); err != nil {
		return err
	}
	if err := validApikeyDesc(r.Desc); err != nil {
		return err
	}
	return nil
}

func (r reqApikeyCheck) valid() error {
	if err := validRank(r.Roles); err != nil {
		return err
	}
	if err := validScope(r.Scope); err != nil {
		return err
	}
	return nil
}
