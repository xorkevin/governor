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

func (r reqUserAuth) valid() error {
	if err := validhasUsernameOrEmail(r.Username); err != nil {
		return err
	}
	if err := validhasPassword(r.Password); err != nil {
		return err
	}
	if err := validOTPCode(r.Code); err != nil {
		return err
	}
	if err := validOTPCode(r.Backup); err != nil {
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

func (r reqUserPost) valid() error {
	if err := validUsername(r.Username); err != nil {
		return err
	}
	if err := validPassword(r.Password); err != nil {
		return err
	}
	if err := validEmail(r.Email); err != nil {
		return err
	}
	if err := validFirstName(r.FirstName); err != nil {
		return err
	}
	if err := validLastName(r.LastName); err != nil {
		return err
	}
	return nil
}

func (r reqUserPostConfirm) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validhasToken(r.Key); err != nil {
		return err
	}
	return nil
}

func (r reqUserDeleteSelf) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validhasUsername(r.Username); err != nil {
		return err
	}
	if err := validhasPassword(r.Password); err != nil {
		return err
	}
	return nil
}

func (r reqUserDelete) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validhasUsername(r.Username); err != nil {
		return err
	}
	return nil
}

func (r reqAddAdmin) valid() error {
	if err := validUsername(r.Username); err != nil {
		return err
	}
	if err := validPassword(r.Password); err != nil {
		return err
	}
	if err := validEmail(r.Email); err != nil {
		return err
	}
	if err := validFirstName(r.Firstname); err != nil {
		return err
	}
	if err := validLastName(r.Lastname); err != nil {
		return err
	}
	return nil
}

func (r reqGetUserApprovals) valid() error {
	if err := validAmount(r.Amount); err != nil {
		return err
	}
	if err := validOffset(r.Offset); err != nil {
		return err
	}
	return nil
}

func (r reqUserPut) valid() error {
	if err := validUsername(r.Username); err != nil {
		return err
	}
	if err := validFirstName(r.FirstName); err != nil {
		return err
	}
	if err := validLastName(r.LastName); err != nil {
		return err
	}
	return nil
}

func (r reqUserPutRank) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validRank(r.Add); err != nil {
		return err
	}
	if err := validRank(r.Remove); err != nil {
		return err
	}
	return nil
}

func (r reqAcceptRoleInvitation) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validhasRole(r.Role); err != nil {
		return err
	}
	return nil
}

func (r reqGetRoleInvitations) valid() error {
	if err := validhasRole(r.Role); err != nil {
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

func (r reqGetUserRoleInvitations) valid() error {
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

func (r reqDelRoleInvitation) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validhasRole(r.Role); err != nil {
		return err
	}
	return nil
}

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

func (r reqUserGetID) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	return nil
}

func (r reqUserGetUsername) valid() error {
	if err := validhasUsername(r.Username); err != nil {
		return err
	}
	return nil
}

func (r reqGetUserRoles) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validhasRolePrefix(r.Prefix); err != nil {
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

func (r reqGetUserRolesIntersect) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validRank(r.Roles); err != nil {
		return err
	}
	return nil
}

func (r reqGetRoleUser) valid() error {
	if err := validhasRole(r.Role); err != nil {
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

func (r reqGetUserBulk) valid() error {
	if err := validAmount(r.Amount); err != nil {
		return err
	}
	if err := validOffset(r.Offset); err != nil {
		return err
	}
	return nil
}

func (r reqGetUsers) valid() error {
	if err := validhasUserids(r.Userids); err != nil {
		return err
	}
	return nil
}

func (r reqSearchUsers) valid() error {
	if err := validoptUsername(r.Prefix); err != nil {
		return err
	}
	if err := validAmount(r.Amount); err != nil {
		return err
	}
	return nil
}

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
