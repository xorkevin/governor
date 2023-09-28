package user

import (
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/gate"
)

type (
	//forge:valid
	reqUserPutEmail struct {
		Userid string `valid:"userid,has" json:"-"`
		Email  string `valid:"email" json:"email"`
	}
)

func (s *router) putEmail(c *governor.Context) {
	var req reqUserPutEmail
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := s.s.updateEmail(c.Ctx(), req.Userid, req.Email); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	//forge:valid
	reqUserPutEmailVerify struct {
		Userid string `valid:"userid,has" json:"userid"`
		Key    string `valid:"token,has" json:"key"`
	}
)

func (s *router) putEmailVerify(c *governor.Context) {
	var req reqUserPutEmailVerify
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := s.s.commitEmail(c.Ctx(), req.Userid, req.Key); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	//forge:valid
	reqUserPutPassword struct {
		Userid      string `valid:"userid,has" json:"-"`
		NewPassword string `valid:"password" json:"new_password"`
		OldPassword string `valid:"password,has" json:"old_password"`
	}
)

func (s *router) putPassword(c *governor.Context) {
	var req reqUserPutPassword
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := s.s.updatePassword(c.Ctx(), req.Userid, req.NewPassword, req.OldPassword); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	//forge:valid
	reqForgotPassword struct {
		Username string `valid:"username,opt" json:"username"`
		Email    string `valid:"email,opt" json:"email"`
	}
)

func (r *reqForgotPassword) preValidate() error {
	if err := r.valid(); err != nil {
		return err
	}
	if r.Username == "" && r.Email == "" {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Must provide either username and email")
	}
	if r.Username != "" && r.Email != "" {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "May not provide both username and email")
	}
	return nil
}

func (s *router) forgotPassword(c *governor.Context) {
	var req reqForgotPassword
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.preValidate(); err != nil {
		c.WriteError(err)
		return
	}

	userid, err := s.s.getUseridForLogin(c.Ctx(), req.Username, req.Email)
	if err != nil {
		c.WriteError(err)
		return
	}
	if err := s.s.forgotPassword(c.Ctx(), userid); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	//forge:valid
	reqForgotPasswordReset struct {
		Userid      string `valid:"userid,has" json:"userid"`
		Key         string `valid:"token,has" json:"key"`
		NewPassword string `valid:"password" json:"new_password"`
	}
)

func (s *router) forgotPasswordReset(c *governor.Context) {
	var req reqForgotPasswordReset
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := s.s.resetPassword(c.Ctx(), req.Userid, req.Key, req.NewPassword); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	//forge:valid
	reqAddOTP struct {
		Userid string `valid:"userid,has" json:"-"`
		Alg    string `valid:"OTPAlg" json:"alg"`
		Digits int    `valid:"OTPDigits" json:"digits"`
	}
)

func (s *router) addOTP(c *governor.Context) {
	var req reqAddOTP
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.addOTP(c.Ctx(), req.Userid, req.Alg, req.Digits)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqOTPCode struct {
		Userid string `valid:"userid,has" json:"-"`
		Code   string `valid:"OTPCode,opt" json:"code"`
	}
)

func (s *router) commitOTP(c *governor.Context) {
	var req reqOTPCode
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := s.s.commitOTP(c.Ctx(), req.Userid, req.Code); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	//forge:valid
	reqOTPCodeBackup struct {
		Userid string `valid:"userid,has" json:"-"`
		Code   string `valid:"OTPCode,opt" json:"code"`
		Backup string `valid:"OTPCode,opt" json:"backup"`
	}
)

func (r *reqOTPCodeBackup) validCode() error {
	if err := r.valid(); err != nil {
		return err
	}
	if r.Code == "" && r.Backup == "" {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "OTP code must be provided")
	}
	if r.Code != "" && r.Backup != "" {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "May not provide both otp code and backup code")
	}
	return nil
}

func (s *router) removeOTP(c *governor.Context) {
	var req reqOTPCodeBackup
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	if err := req.validCode(); err != nil {
		c.WriteError(err)
		return
	}

	var ipaddr string
	if ip := c.RealIP(); ip != nil {
		ipaddr = ip.String()
	}
	if err := s.s.removeOTP(c.Ctx(), req.Userid, req.Code, req.Backup, ipaddr, c.Header("User-Agent")); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (s *router) mountEditSecure(m *governor.MethodRouter) {
	m.PutCtx("/email", s.putEmail, gate.AuthUserSudo(s.s.gate, s.s.authSettings.sudoDuration, gate.ScopeNone), s.rt)
	m.PutCtx("/email/verify", s.putEmailVerify, gate.AuthUserSudo(s.s.gate, s.s.authSettings.sudoDuration, gate.ScopeNone), s.rt)
	m.PutCtx("/password", s.putPassword, gate.AuthUserSudo(s.s.gate, s.s.authSettings.sudoDuration, gate.ScopeNone), s.rt)
	m.PutCtx("/password/forgot", s.forgotPassword, s.rt)
	m.PutCtx("/password/forgot/reset", s.forgotPasswordReset, s.rt)
	m.PutCtx("/otp", s.addOTP, gate.AuthUserSudo(s.s.gate, s.s.authSettings.sudoDuration, gate.ScopeNone), s.rt)
	m.PutCtx("/otp/verify", s.commitOTP, gate.AuthUserSudo(s.s.gate, s.s.authSettings.sudoDuration, gate.ScopeNone), s.rt)
	m.DeleteCtx("/otp", s.removeOTP, gate.AuthUserSudo(s.s.gate, s.s.authSettings.sudoDuration, gate.ScopeNone), s.rt)
}
