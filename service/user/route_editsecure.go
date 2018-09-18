package user

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/gate"
	"github.com/hackform/governor/service/user/model"
	"github.com/hackform/governor/util/uid"
	"github.com/labstack/echo"
	"net/http"
	"time"
)

type (
	emailForgotPass struct {
		FirstName string
		Username  string
		Key       string
	}

	emailPassReset struct {
		FirstName string
		Username  string
	}

	emailPassChange struct {
		FirstName string
		Username  string
		Key       string
	}

	emailEmailChange struct {
		FirstName string
		Username  string
		Key       string
	}

	emailEmailChangeNotify struct {
		FirstName string
		Username  string
	}
)

const (
	forgotPassTemplate        = "forgotpass"
	forgotPassSubject         = "forgotpass_subject"
	passResetTemplate         = "passreset"
	passResetSubject          = "passreset_subject"
	passChangeTemplate        = "passchange"
	passChangeSubject         = "passchange_subject"
	emailChangeTemplate       = "emailchange"
	emailChangeSubject        = "emailchange_subject"
	emailChangeNotifyTemplate = "emailchangenotify"
	emailChangeNotifySubject  = "emailchangenotify_subject"
)

const (
	emailChangeEscapeSequence = "%email%"
)

type (
	reqUserPutEmail struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	reqUserPutEmailVerify struct {
		Key      string `json:"key"`
		Password string `json:"password"`
	}
)

func (u *userRouter) putEmail(c echo.Context) error {
	userid := c.Get("userid").(string)

	ruser := reqUserPutEmail{}
	if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	if err := u.service.UpdateEmail(userid, ruser.Email, ruser.Password); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (r *reqUserPutEmail) valid() *governor.Error {
	if err := validEmail(r.Email); err != nil {
		return err
	}
	if err := hasPassword(r.Password); err != nil {
		return err
	}
	return nil
}

func (r *reqUserPutEmailVerify) valid() *governor.Error {
	if err := hasToken(r.Key); err != nil {
		return err
	}
	if err := hasPassword(r.Password); err != nil {
		return err
	}
	return nil
}

func (u *userRouter) putEmailVerify(c echo.Context) error {
	ruser := reqUserPutEmailVerify{}
	if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	if err := u.service.CommitEmail(ruser.Key, ruser.Password); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

type (
	reqUserPutPassword struct {
		NewPassword string `json:"new_password"`
		OldPassword string `json:"old_password"`
	}

	reqForgotPassword struct {
		Username string `json:"username"`
	}

	reqForgotPasswordReset struct {
		Key         string `json:"key"`
		NewPassword string `json:"new_password"`
	}
)

func (r *reqUserPutPassword) valid(passlen int) *governor.Error {
	if err := validPassword(r.NewPassword, passlen); err != nil {
		return err
	}
	if err := hasPassword(r.OldPassword); err != nil {
		return err
	}
	return nil
}

func (r *reqForgotPassword) valid() *governor.Error {
	if err := validUsername(r.Username); err != nil {
		return err
	}
	return nil
}

func (r *reqForgotPassword) validEmail() *governor.Error {
	if err := validEmail(r.Username); err != nil {
		return err
	}
	return nil
}

func (r *reqForgotPasswordReset) valid(passlen int) *governor.Error {
	if err := hasToken(r.Key); err != nil {
		return err
	}
	if err := validPassword(r.NewPassword, passlen); err != nil {
		return err
	}
	return nil
}

func (u *userRouter) putPassword(c echo.Context) error {
	db := u.service.db.DB()
	ch := u.service.cache.Cache()
	mailer := u.service.mailer

	userid := c.Get("userid").(string)

	ruser := reqUserPutPassword{}
	if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
	}
	if err := ruser.valid(u.service.passwordMinSize); err != nil {
		return err
	}

	m, err := usermodel.GetByIDB64(db, userid)
	if err != nil {
		if err.Code() == 2 {
			err.SetErrorUser()
		}
		err.AddTrace(moduleIDUser)
		return err
	}
	if !m.ValidatePass(ruser.OldPassword) {
		return governor.NewErrorUser(moduleIDUser, "incorrect password", 0, http.StatusForbidden)
	}
	if err = m.RehashPass(ruser.NewPassword); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	key, err := uid.NewU(0, 16)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	sessionKey := key.Base64()

	if err := ch.Set(sessionKey, userid, time.Duration(u.service.passwordResetTime*b1)).Err(); err != nil {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	emdata := emailPassChange{
		FirstName: m.FirstName,
		Username:  m.Username,
		Key:       sessionKey,
	}

	em, err := u.service.tpl.ExecuteHTML(passChangeTemplate, emdata)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	subj, err := u.service.tpl.ExecuteHTML(passChangeSubject, emdata)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	if err := mailer.Send(m.Email, subj, em); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	if err = m.Update(db); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (u *userRouter) forgotPassword(c echo.Context) error {
	db := u.service.db.DB()
	ch := u.service.cache.Cache()
	mailer := u.service.mailer

	ruser := reqForgotPassword{}
	if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
	}
	isEmail := false
	if err := ruser.validEmail(); err == nil {
		isEmail = true
	}
	var m *usermodel.Model
	if isEmail {
		mu, err := usermodel.GetByEmail(db, ruser.Username)
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleIDUser)
			return err
		}
		m = mu
	} else {
		if err := ruser.valid(); err != nil {
			return err
		}
		mu, err := usermodel.GetByUsername(db, ruser.Username)
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleIDUser)
			return err
		}
		m = mu
	}

	key, err := uid.NewU(0, 16)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	sessionKey := key.Base64()

	userid, err := m.IDBase64()
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	if err := ch.Set(sessionKey, userid, time.Duration(u.service.passwordResetTime*b1)).Err(); err != nil {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	emdata := emailForgotPass{
		FirstName: m.FirstName,
		Username:  m.Username,
		Key:       sessionKey,
	}

	em, err := u.service.tpl.ExecuteHTML(forgotPassTemplate, emdata)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	subj, err := u.service.tpl.ExecuteHTML(forgotPassSubject, emdata)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	if err := mailer.Send(m.Email, subj, em); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

func (u *userRouter) forgotPasswordReset(c echo.Context) error {
	db := u.service.db.DB()
	ch := u.service.cache.Cache()
	mailer := u.service.mailer

	ruser := reqForgotPasswordReset{}
	if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
	}
	if err := ruser.valid(u.service.passwordMinSize); err != nil {
		return err
	}

	userid := ""
	if result, err := ch.Get(ruser.Key).Result(); err == nil {
		userid = result
	} else {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	m, err := usermodel.GetByIDB64(db, userid)
	if err != nil {
		if err.Code() == 2 {
			err.SetErrorUser()
		}
		err.AddTrace(moduleIDUser)
		return err
	}

	if err := m.RehashPass(ruser.NewPassword); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	emdata := emailPassReset{
		FirstName: m.FirstName,
		Username:  m.Username,
	}

	em, err := u.service.tpl.ExecuteHTML(passResetTemplate, emdata)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	subj, err := u.service.tpl.ExecuteHTML(passResetSubject, emdata)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	if err := mailer.Send(m.Email, subj, em); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	if err := ch.Del(ruser.Key).Err(); err != nil {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	if err := m.Update(db); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

func (u *userRouter) mountEditSecure(conf governor.Config, r *echo.Group) error {
	r.PUT("/email", u.putEmail, gate.User(u.service.gate))
	r.PUT("/email/verify", u.putEmailVerify)
	r.PUT("/password", u.putPassword, gate.User(u.service.gate))
	r.PUT("/password/forgot", u.forgotPassword)
	r.PUT("/password/forgot/reset", u.forgotPasswordReset)
	return nil
}
