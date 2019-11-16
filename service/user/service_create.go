package user

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
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
		Email     string
		Key       string
		FirstName string
		Username  string
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

func prefixEmailKey(email string) string {
	return "nonce:" + email
}

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
	noncehash, err := s.hasher.Hash(nonce)
	if err != nil {
		return nil, governor.NewError("Failed to hash email validation key", http.StatusInternalServerError, err)
	}

	if err := s.kvnewuser.Set(prefixEmailKey(m.Email), noncehash, s.confirmTime); err != nil {
		return nil, governor.NewError("Failed to store email validation key", http.StatusInternalServerError, err)
	}
	if err := s.kvnewuser.Set(m.Email, b.String(), s.confirmTime); err != nil {
		return nil, governor.NewError("Failed to store new user info", http.StatusInternalServerError, err)
	}

	emdata := emailNewUser{
		Email:     m.Email,
		Key:       nonce,
		FirstName: m.FirstName,
		Username:  m.Username,
	}
	if err := s.mailer.Send("", "", m.Email, newUserSubject, newUserTemplate, emdata); err != nil {
		return nil, governor.NewError("Failed to send account verification email", http.StatusInternalServerError, err)
	}

	return &resUserUpdate{
		Userid:   m.Userid,
		Username: m.Username,
	}, nil
}

// CommitUser takes a user from the cache and places it into the db
func (s *service) CommitUser(email string, key string) (*resUserUpdate, error) {
	noncehash, err := s.kvnewuser.Get(prefixEmailKey(email))
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, governor.NewErrorUser("Account verification expired", http.StatusBadRequest, err)
		}
		return nil, governor.NewError("Failed to get account", http.StatusInternalServerError, err)
	}
	if ok, err := s.verifier.Verify(key, noncehash); err != nil {
		return nil, governor.NewError("Failed to verify key", http.StatusInternalServerError, err)
	} else if !ok {
		return nil, governor.NewErrorUser("Invalid key", http.StatusForbidden, nil)
	}

	gobUser, err := s.kvnewuser.Get(email)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, governor.NewErrorUser("Account verification expired", http.StatusBadRequest, err)
		}
		return nil, governor.NewError("Failed to get user info", http.StatusInternalServerError, err)
	}

	m := s.users.NewEmpty()
	if err := gob.NewDecoder(bytes.NewBufferString(gobUser)).Decode(&m); err != nil {
		return nil, governor.NewError("Failed to decode user info", http.StatusInternalServerError, err)
	}

	userProps := NewUserProps{
		Userid:       m.Userid,
		Username:     m.Username,
		Email:        m.Email,
		FirstName:    m.FirstName,
		LastName:     m.LastName,
		CreationTime: m.CreationTime,
	}

	b := bytes.Buffer{}
	if err := json.NewEncoder(&b).Encode(userProps); err != nil {
		return nil, governor.NewError("Failed to encode user props to json", http.StatusInternalServerError, err)
	}

	if err := s.users.Insert(&m); err != nil {
		if governor.ErrorStatus(err) == http.StatusBadRequest {
			return nil, governor.NewErrorUser("", 0, err)
		}
		return nil, err
	}

	if err := s.queue.Publish(NewUserQueueID, b.Bytes()); err != nil {
		s.logger.Error("failed to publish new user", map[string]string{
			"error":      err.Error(),
			"actiontype": "publishnewuser",
		})
	}

	if err := s.kvnewuser.Del(prefixEmailKey(email), email); err != nil {
		s.logger.Error("failed to clean up new user info", map[string]string{
			"error":      err.Error(),
			"actiontype": "commitusercleanup",
		})
	}

	s.logger.Info("created user", map[string]string{
		"userid":     m.Userid,
		"username":   m.Username,
		"actiontype": "commituser",
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

	userProps := DeleteUserProps{
		Userid: m.Userid,
	}
	b := bytes.Buffer{}
	if err := json.NewEncoder(&b).Encode(userProps); err != nil {
		return governor.NewError("Failed to encode user props to json", http.StatusInternalServerError, err)
	}

	if err := s.KillAllSessions(userid); err != nil {
		return err
	}

	if err := s.users.Delete(m); err != nil {
		return err
	}

	if err := s.queue.Publish(DeleteUserQueueID, b.Bytes()); err != nil {
		s.logger.Error("failed to publish delete user", map[string]string{
			"error":      err.Error(),
			"actiontype": "publishdeleteuser",
		})
	}
	return nil
}

// DecodeNewUserProps marshals json encoded new user props into a struct
func DecodeNewUserProps(msgdata []byte) (*NewUserProps, error) {
	m := &NewUserProps{}
	if err := json.NewDecoder(bytes.NewBuffer(msgdata)).Decode(m); err != nil {
		return nil, governor.NewError("Failed to decode new user props", http.StatusInternalServerError, err)
	}
	return m, nil
}

// DecodeDeleteUserProps marshals json encoded delete user props into a struct
func DecodeDeleteUserProps(msgdata []byte) (*DeleteUserProps, error) {
	m := &DeleteUserProps{}
	if err := json.NewDecoder(bytes.NewBuffer(msgdata)).Decode(m); err != nil {
		return nil, governor.NewError("Failed to decode delete user props", http.StatusInternalServerError, err)
	}
	return m, nil
}
