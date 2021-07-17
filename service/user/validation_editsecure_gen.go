// Code generated by go generate forge validation v0.3; DO NOT EDIT.

package user

func (r reqUserPutEmail) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validEmail(r.Email); err != nil {
		return err
	}
	if err := validhasPassword(r.Password); err != nil {
		return err
	}
	return nil
}

func (r reqUserPutEmailVerify) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validhasToken(r.Key); err != nil {
		return err
	}
	if err := validhasPassword(r.Password); err != nil {
		return err
	}
	return nil
}

func (r reqUserPutPassword) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validPassword(r.NewPassword); err != nil {
		return err
	}
	if err := validhasPassword(r.OldPassword); err != nil {
		return err
	}
	return nil
}

func (r reqForgotPassword) valid() error {
	if err := validhasUsernameOrEmail(r.Username); err != nil {
		return err
	}
	return nil
}

func (r reqForgotPasswordReset) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validhasToken(r.Key); err != nil {
		return err
	}
	if err := validPassword(r.NewPassword); err != nil {
		return err
	}
	return nil
}

func (r reqAddOTP) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validOTPAlg(r.Alg); err != nil {
		return err
	}
	if err := validOTPDigits(r.Digits); err != nil {
		return err
	}
	if err := validhasPassword(r.Password); err != nil {
		return err
	}
	return nil
}

func (r reqOTPCode) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validOTPCode(r.Code); err != nil {
		return err
	}
	return nil
}

func (r reqOTPCodeBackup) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validOTPCode(r.Code); err != nil {
		return err
	}
	if err := validOTPCode(r.Backup); err != nil {
		return err
	}
	if err := validhasPassword(r.Password); err != nil {
		return err
	}
	return nil
}
