package user

import (
	"bytes"
	"encoding/gob"
	"github.com/hackform/governor"
	"github.com/hackform/governor/util/rank"
	"github.com/hackform/governor/util/uid"
	"net/http"
	"time"
)

type (
	emailNewUser struct {
		FirstName string
		Username  string
		Key       string
	}
)

const (
	newUserTemplate = "newuser"
	newUserSubject  = "newuser_subject"
)

type (
	resUserUpdate struct {
		Userid   string `json:"userid"`
		Username string `json:"username"`
	}
)

const (
	cachePrefixNewUser = moduleID + ".newuser:"
)

// CreateUser creates a new user and places it into the cache
func (u *userService) CreateUser(ruser reqUserPost) (*resUserUpdate, *governor.Error) {
	m2, err := u.repo.GetByUsername(ruser.Username)
	if err != nil && err.Code() != 2 {
		err.AddTrace(moduleIDUser)
		return nil, err
	}
	if m2 != nil && m2.Username == ruser.Username {
		return nil, governor.NewErrorUser(moduleIDUser, "username is already taken", 0, http.StatusBadRequest)
	}

	m2, err = u.repo.GetByEmail(ruser.Email)
	if err != nil && err.Code() != 2 {
		err.AddTrace(moduleIDUser)
		return nil, err
	}
	if m2 != nil && m2.Email == ruser.Email {
		return nil, governor.NewErrorUser(moduleIDUser, "email is already used by another account", 0, http.StatusBadRequest)
	}

	m, err := u.repo.New(ruser.Username, ruser.Password, ruser.Email, ruser.FirstName, ruser.LastName, rank.BaseUser())
	if err != nil {
		err.AddTrace(moduleIDUser)
		return nil, err
	}

	b := bytes.Buffer{}
	if err := gob.NewEncoder(&b).Encode(m); err != nil {
		return nil, governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	key, err := uid.NewU(0, 16)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return nil, err
	}
	sessionKey := key.Base64()

	if err := u.cache.Cache().Set(cachePrefixNewUser+sessionKey, b.String(), time.Duration(u.confirmTime*b1)).Err(); err != nil {
		return nil, governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	emdata := emailNewUser{
		FirstName: m.FirstName,
		Username:  m.Username,
		Key:       sessionKey,
	}
	if err := u.mailer.Send(m.Email, newUserSubject, newUserTemplate, emdata); err != nil {
		err.AddTrace(moduleIDUser)
		return nil, err
	}

	userid, _ := m.IDBase64()
	return &resUserUpdate{
		Userid:   userid,
		Username: m.Username,
	}, nil
}

// CommitUser takes a user from the cache and places it into the db
func (u *userService) CommitUser(key string) (*resUserUpdate, *governor.Error) {
	gobUser, err := u.cache.Cache().Get(cachePrefixNewUser + key).Result()
	if err != nil {
		return nil, governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
	}

	m := u.repo.NewEmpty()
	b := bytes.NewBufferString(gobUser)
	if err := gob.NewDecoder(b).Decode(&m); err != nil {
		return nil, governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	if err := u.repo.Insert(&m); err != nil {
		if err.Code() == 3 {
			err.SetErrorUser()
		}
		err.AddTrace(moduleIDUser)
		return nil, err
	}

	if err := u.cache.Cache().Del(cachePrefixNewUser + key).Err(); err != nil {
		return nil, governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	userid, _ := m.IDBase64()
	hookProps := HookProps{
		Userid:    userid,
		Username:  m.Username,
		Email:     m.Email,
		FirstName: m.FirstName,
		LastName:  m.LastName,
	}

	for _, i := range u.hooks {
		if err := i.UserCreateHook(hookProps); err != nil {
			err.AddTrace(moduleIDUser)
			u.logger.Error(err.Message(), err.Origin(), "userhook create error", err.Code(), map[string]string{
				"source": err.Source(),
			})
		}
	}

	t, _ := time.Now().MarshalText()
	u.logger.Info("user created", moduleIDUser, "create user", 0, map[string]string{
		"userid":   userid,
		"username": m.Username,
	})

	return &resUserUpdate{
		Userid:   userid,
		Username: m.Username,
	}, nil
}

func (u *userService) DeleteUser(userid string, username string, password string) *governor.Error {
	m, err := u.repo.GetByIDB64(userid)
	if err != nil {
		if err.Code() == 2 {
			err.SetErrorUser()
		}
		err.AddTrace(moduleIDUser)
		return err
	}

	if m.Username != username {
		return governor.NewErrorUser(moduleIDUser, "information does not match", 0, http.StatusBadRequest)
	}
	if !m.ValidatePass(password) {
		return governor.NewErrorUser(moduleIDUser, "incorrect password", 0, http.StatusForbidden)
	}

	if err := u.KillAllSessions(userid); err != nil {
		return err
	}

	if err := u.rolerepo.DeleteUserRoles(userid); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	if err := u.repo.Delete(m); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	for _, i := range u.hooks {
		if err := i.UserDeleteHook(userid); err != nil {
			err.AddTrace(moduleIDUser)
			u.logger.Error(err.Message(), err.Origin(), "userhook delete error", err.Code(), map[string]string{
				"source": err.Source(),
			})
		}
	}
	return nil
}
