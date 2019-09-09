package user

import (
	"bytes"
	"encoding/gob"
	"net/http"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/governor/util/uid"
)

const (
	uidUserSize = 16
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

// CreateUser creates a new user and places it into the cache
func (s *service) CreateUser(ruser reqUserPost) (*resUserUpdate, error) {
	m2, err := s.users.GetByUsername(ruser.Username)
	if err != nil && governor.ErrorStatus(err) != http.StatusNotFound {
		return nil, err
	}
	if m2 != nil && m2.Username == ruser.Username {
		return nil, governor.NewErrorUser("Username is already taken", http.StatusBadRequest, nil)
	}

	m2, err = s.users.GetByEmail(ruser.Email)
	if err != nil && governor.ErrorStatus(err) != http.StatusNotFound {
		return nil, err
	}
	if m2 != nil && m2.Email == ruser.Email {
		return nil, governor.NewErrorUser("Email is already used by another account", http.StatusBadRequest, nil)
	}

	m, err := s.users.New(ruser.Username, ruser.Password, ruser.Email, ruser.FirstName, ruser.LastName, rank.BaseUser())
	if err != nil {
		return nil, err
	}

	b := bytes.Buffer{}
	if err := gob.NewEncoder(&b).Encode(m); err != nil {
		return nil, governor.NewError("Failed to encode user info", http.StatusInternalServerError, err)
	}

	key, err := uid.New(uidUserSize)
	if err != nil {
		return nil, governor.NewError("Failed to create new uid", http.StatusInternalServerError, err)
	}
	nonce := key.Base64()

	if err := s.kvnewuser.Set(nonce, b.String(), s.confirmTime); err != nil {
		return nil, governor.NewError("Failed to store user info", http.StatusInternalServerError, err)
	}

	emdata := emailNewUser{
		FirstName: m.FirstName,
		Username:  m.Username,
		Key:       nonce,
	}
	if err := s.mailer.Send(m.Email, newUserSubject, newUserTemplate, emdata); err != nil {
		return nil, governor.NewError("Failed to send account verification email", http.StatusInternalServerError, err)
	}

	return &resUserUpdate{
		Userid:   m.Userid,
		Username: m.Username,
	}, nil
}

// CommitUser takes a user from the cache and places it into the db
func (s *service) CommitUser(key string) (*resUserUpdate, error) {
	gobUser, err := s.kvnewuser.Get(key)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, governor.NewErrorUser("Account verification expired", http.StatusBadRequest, err)
		}
		return nil, governor.NewError("Failed to get account", http.StatusInternalServerError, err)
	}

	m := s.users.NewEmpty()
	b := bytes.NewBufferString(gobUser)
	if err := gob.NewDecoder(b).Decode(&m); err != nil {
		return nil, governor.NewError("Failed to decode user info", http.StatusInternalServerError, err)
	}

	if err := s.users.Insert(&m); err != nil {
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

	for _, i := range s.hooks {
		if err := i.UserCreateHook(hookProps); err != nil {
			s.logger.Error("user: userhook_create error", map[string]string{
				"error": err.Error(),
			})
		}
	}

	if err := s.kvnewuser.Del(key); err != nil {
		s.logger.Error("user: failed to clean up user create cache data", map[string]string{
			"error": err.Error(),
		})
	}

	s.logger.Info("user: created user", map[string]string{
		"userid":   m.Userid,
		"username": m.Username,
	})

	return &resUserUpdate{
		Userid:   m.Userid,
		Username: m.Username,
	}, nil
}

func (s *service) DeleteUser(userid string, username string, password string) error {
	m, err := s.users.GetByID(userid)
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
	if ok, err := s.users.ValidatePass(password, m); err != nil {
		return err
	} else if !ok {
		return governor.NewErrorUser("Incorrect password", http.StatusForbidden, nil)
	}

	if err := s.KillAllSessions(userid); err != nil {
		return err
	}

	if err := s.users.Delete(m); err != nil {
		return err
	}

	for _, i := range s.hooks {
		if err := i.UserDeleteHook(userid); err != nil {
			s.logger.Error("user: userhook_delete error", map[string]string{
				"error": err.Error(),
			})
		}
	}
	return nil
}
