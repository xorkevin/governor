package user

import (
	"bytes"
	"encoding/gob"
	"net/http"
	"time"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/governor/util/uid"
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
func (u *userService) CreateUser(ruser reqUserPost) (*resUserUpdate, error) {
	m2, err := u.repo.GetByUsername(ruser.Username)
	if err != nil && governor.ErrorStatus(err) != http.StatusNotFound {
		return nil, err
	}
	if m2 != nil && m2.Username == ruser.Username {
		return nil, governor.NewErrorUser("Username is already taken", http.StatusBadRequest, nil)
	}

	m2, err = u.repo.GetByEmail(ruser.Email)
	if err != nil && governor.ErrorStatus(err) != http.StatusNotFound {
		return nil, err
	}
	if m2 != nil && m2.Email == ruser.Email {
		return nil, governor.NewErrorUser("Email is already used by another account", http.StatusBadRequest, nil)
	}

	m, err := u.repo.New(ruser.Username, ruser.Password, ruser.Email, ruser.FirstName, ruser.LastName, rank.BaseUser())
	if err != nil {
		return nil, err
	}

	b := bytes.Buffer{}
	if err := gob.NewEncoder(&b).Encode(m); err != nil {
		return nil, governor.NewError("Failed to encode user info", http.StatusInternalServerError, err)
	}

	key, err := uid.NewU(0, 16)
	if err != nil {
		return nil, governor.NewError("Failed to create new uid", http.StatusInternalServerError, err)
	}
	sessionKey := key.Base64()

	if err := u.cache.Cache().Set(cachePrefixNewUser+sessionKey, b.String(), time.Duration(u.confirmTime*b1)).Err(); err != nil {
		return nil, governor.NewError("Failed to store user info", http.StatusInternalServerError, err)
	}

	emdata := emailNewUser{
		FirstName: m.FirstName,
		Username:  m.Username,
		Key:       sessionKey,
	}
	if err := u.mailer.Send(m.Email, newUserSubject, newUserTemplate, emdata); err != nil {
		return nil, governor.NewError("Failed to send account verification email", http.StatusInternalServerError, err)
	}

	return &resUserUpdate{
		Userid:   m.Userid,
		Username: m.Username,
	}, nil
}

// CommitUser takes a user from the cache and places it into the db
func (u *userService) CommitUser(key string) (*resUserUpdate, error) {
	gobUser, err := u.cache.Cache().Get(cachePrefixNewUser + key).Result()
	if err != nil {
		return nil, governor.NewErrorUser("Account verification expired", http.StatusBadRequest, err)
	}

	m := u.repo.NewEmpty()
	b := bytes.NewBufferString(gobUser)
	if err := gob.NewDecoder(b).Decode(&m); err != nil {
		return nil, governor.NewError("Failed to decode user info", http.StatusInternalServerError, err)
	}

	if err := u.repo.Insert(&m); err != nil {
		if governor.ErrorStatus(err) == http.StatusBadRequest {
			return nil, governor.NewErrorUser("", 0, err)
		}
		return nil, err
	}

	hookProps := HookProps{
		Userid:    m.Userid,
		Username:  m.Username,
		Email:     m.Email,
		FirstName: m.FirstName,
		LastName:  m.LastName,
	}

	for _, i := range u.hooks {
		if err := i.UserCreateHook(hookProps); err != nil {
			u.logger.Error("userhook create error", map[string]string{
				"err": err.Error(),
			})
		}
	}

	if err := u.cache.Cache().Del(cachePrefixNewUser + key).Err(); err != nil {
		u.logger.Error("Failed to clean up user create cache data", map[string]string{
			"err": err.Error(),
		})
	}

	u.logger.Info("create user", map[string]string{
		"userid":   m.Userid,
		"username": m.Username,
	})

	return &resUserUpdate{
		Userid:   m.Userid,
		Username: m.Username,
	}, nil
}

func (u *userService) DeleteUser(userid string, username string, password string) error {
	m, err := u.repo.GetByID(userid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("", 0, err)
		}
		return err
	}

	if m.Username != username {
		return governor.NewErrorUser("Information does not match", http.StatusBadRequest, err)
	}
	if m.AuthTags.Has("admin") {
		return governor.NewErrorUser("Not allowed to delete admin user", http.StatusForbidden, err)
	}
	if ok, err := u.repo.ValidatePass(password, m); err != nil {
		return err
	} else if !ok {
		return governor.NewErrorUser("Incorrect password", http.StatusForbidden, nil)
	}

	if err := u.KillAllSessions(userid); err != nil {
		return err
	}

	if err := u.repo.Delete(m); err != nil {
		return err
	}

	for _, i := range u.hooks {
		if err := i.UserDeleteHook(userid); err != nil {
			u.logger.Error("userhook delete error", map[string]string{
				"err": err.Error(),
			})
		}
	}
	return nil
}
