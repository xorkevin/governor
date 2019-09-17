// Code generated by go generate forge validation v0.2.0. DO NOT EDIT.
package user

func (r reqUserAuth) valid() error {
	if err := validhasPassword(r.Password); err != nil {
		return err
	}
	if err := validhasSessionToken(r.SessionToken); err != nil {
		return err
	}
	return nil
}

func (r reqRefreshToken) valid() error {
	if err := validhasRefreshToken(r.RefreshToken); err != nil {
		return err
	}
	return nil
}
