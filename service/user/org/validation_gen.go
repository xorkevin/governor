// Code generated by go generate forge validation v0.5.2; DO NOT EDIT.

package org

func (r reqOrgGet) valid() error {
	if err := validhasOrgid(r.OrgID); err != nil {
		return err
	}
	return nil
}

func (r reqOrgNameGet) valid() error {
	if err := validhasName(r.Name); err != nil {
		return err
	}
	return nil
}

func (r reqOrgsGet) valid() error {
	if err := validhasOrgids(r.OrgIDs); err != nil {
		return err
	}
	return nil
}

func (r reqOrgMembersSearch) valid() error {
	if err := validhasOrgid(r.OrgID); err != nil {
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

func (r reqOrgsSearch) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validoptName(r.Prefix); err != nil {
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

func (r reqOrgsGetBulk) valid() error {
	if err := validAmount(r.Amount); err != nil {
		return err
	}
	if err := validOffset(r.Offset); err != nil {
		return err
	}
	return nil
}

func (r reqOrgPost) valid() error {
	if err := validDisplay(r.Display); err != nil {
		return err
	}
	if err := validDesc(r.Desc); err != nil {
		return err
	}
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	return nil
}

func (r reqOrgPut) valid() error {
	if err := validhasOrgid(r.OrgID); err != nil {
		return err
	}
	if err := validName(r.Name); err != nil {
		return err
	}
	if err := validDisplay(r.Display); err != nil {
		return err
	}
	if err := validDesc(r.Desc); err != nil {
		return err
	}
	return nil
}
