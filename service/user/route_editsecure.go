package user

import (
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/user/token"
)

//go:generate forge validation -o validation_editsecure_gen.go reqUserPutEmail reqUserPutEmailVerify reqUserPutPassword reqForgotPassword reqForgotPasswordReset reqAddOTP reqOTPCode reqOTPCodeBackup

type (
	reqUserPutEmail struct {
		Userid   string `valid:"userid,has" json:"-"`
		Email    string `valid:"email" json:"email"`
		Password string `valid:"password,has" json:"password"`
	}
)

func (m *router) putEmail(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqUserPutEmail{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := m.s.UpdateEmail(c.Ctx(), req.Userid, req.Email, req.Password); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	reqUserPutEmailVerify struct {
		Userid   string `valid:"userid,has" json:"userid"`
		Key      string `valid:"token,has" json:"key"`
		Password string `valid:"password,has" json:"password"`
	}
)

func (m *router) putEmailVerify(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqUserPutEmailVerify{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := m.s.CommitEmail(c.Ctx(), req.Userid, req.Key, req.Password); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	reqUserPutPassword struct {
		Userid      string `valid:"userid,has" json:"-"`
		NewPassword string `valid:"password" json:"new_password"`
		OldPassword string `valid:"password,has" json:"old_password"`
	}
)

func (m *router) putPassword(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqUserPutPassword{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := m.s.UpdatePassword(c.Ctx(), req.Userid, req.NewPassword, req.OldPassword); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	reqForgotPassword struct {
		Username string `valid:"usernameOrEmail,has" json:"username"`
	}
)

func (m *router) forgotPassword(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqForgotPassword{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := m.s.ForgotPassword(c.Ctx(), req.Username); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	reqForgotPasswordReset struct {
		Userid      string `valid:"userid,has" json:"userid"`
		Key         string `valid:"token,has" json:"key"`
		NewPassword string `valid:"password" json:"new_password"`
	}
)

func (m *router) forgotPasswordReset(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqForgotPasswordReset{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := m.s.ResetPassword(c.Ctx(), req.Userid, req.Key, req.NewPassword); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	reqAddOTP struct {
		Userid   string `valid:"userid,has" json:"-"`
		Alg      string `valid:"OTPAlg" json:"alg"`
		Digits   int    `valid:"OTPDigits" json:"digits"`
		Password string `valid:"password,has" json:"password"`
	}
)

func (m *router) addOTP(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqAddOTP{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.AddOTP(c.Ctx(), req.Userid, req.Alg, req.Digits, req.Password)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	reqOTPCode struct {
		Userid string `valid:"userid,has" json:"-"`
		Code   string `valid:"OTPCode" json:"code"`
	}
)

func (m *router) commitOTP(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqOTPCode{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := m.s.CommitOTP(c.Ctx(), req.Userid, req.Code); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	reqOTPCodeBackup struct {
		Userid   string `valid:"userid,has" json:"-"`
		Code     string `valid:"OTPCode" json:"code"`
		Backup   string `valid:"OTPCode" json:"backup"`
		Password string `valid:"password,has" json:"password"`
	}
)

func (r *reqOTPCodeBackup) validCode() error {
	if err := r.valid(); err != nil {
		return err
	}
	if len(r.Code) == 0 && len(r.Backup) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "OTP code must be provided")
	}
	if len(r.Code) > 0 && len(r.Backup) > 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "May not provide both otp code and backup code")
	}
	return nil
}

func (m *router) removeOTP(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqOTPCodeBackup{}
	if err := c.Bind(&req); err != nil {
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
	if err := m.s.RemoveOTP(c.Ctx(), req.Userid, req.Code, req.Backup, req.Password, ipaddr, c.Header("User-Agent")); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (m *router) mountEditSecure(r governor.Router) {
	r.Put("/email", m.putEmail, gate.User(m.s.gate, token.ScopeForbidden), m.rt)
	r.Put("/email/verify", m.putEmailVerify, m.rt)
	r.Put("/password", m.putPassword, gate.User(m.s.gate, token.ScopeForbidden), m.rt)
	r.Put("/password/forgot", m.forgotPassword, m.rt)
	r.Put("/password/forgot/reset", m.forgotPasswordReset, m.rt)
	r.Put("/otp", m.addOTP, gate.User(m.s.gate, token.ScopeForbidden), m.rt)
	r.Put("/otp/verify", m.commitOTP, gate.User(m.s.gate, token.ScopeForbidden), m.rt)
	r.Delete("/otp", m.removeOTP, gate.User(m.s.gate, token.ScopeForbidden), m.rt)
}
