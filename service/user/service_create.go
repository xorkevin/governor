package user

import (
	"bytes"
	"encoding/json"
	htmlTemplate "html/template"
	"net/http"
	"net/url"
	"time"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/approval/model"
	"xorkevin.dev/governor/util/rank"
)

type (
	emailNewUser struct {
		Userid    string
		Key       string
		URL       string
		FirstName string
		LastName  string
		Username  string
	}

	queryEmailNewUser struct {
		Userid    string
		Key       string
		FirstName string
		LastName  string
		Username  string
	}
)

const (
	newUserTemplate = "newuser"
)

func (e *emailNewUser) Query() queryEmailNewUser {
	return queryEmailNewUser{
		Userid:    url.QueryEscape(e.Userid),
		Key:       url.QueryEscape(e.Key),
		FirstName: url.QueryEscape(e.FirstName),
		LastName:  url.QueryEscape(e.LastName),
		Username:  url.QueryEscape(e.Username),
	}
}

func (e *emailNewUser) computeURL(base string, tpl *htmlTemplate.Template) error {
	b := &bytes.Buffer{}
	if err := tpl.Execute(b, e.Query()); err != nil {
		return governor.NewError("Failed executing new user url template", http.StatusInternalServerError, err)
	}
	e.URL = base + b.String()
	return nil
}

type (
	resUserUpdate struct {
		Userid   string `json:"userid"`
		Username string `json:"username"`
	}
)

// CreateUser creates a new user and places it into approvals
func (s *service) CreateUser(ruser reqUserPost) (*resUserUpdate, error) {
	if _, err := s.users.GetByUsername(ruser.Username); err != nil {
		if governor.ErrorStatus(err) != http.StatusNotFound {
			return nil, err
		}
	} else {
		return nil, governor.NewErrorUser("Username is already taken", http.StatusBadRequest, nil)
	}

	if _, err := s.users.GetByEmail(ruser.Email); err != nil {
		if governor.ErrorStatus(err) != http.StatusNotFound {
			return nil, err
		}
	} else {
		return nil, governor.NewErrorUser("Email is already used by another account", http.StatusBadRequest, nil)
	}

	m, err := s.users.New(ruser.Username, ruser.Password, ruser.Email, ruser.FirstName, ruser.LastName)
	if err != nil {
		return nil, err
	}

	am := s.approvals.New(m)
	if s.userApproval {
		if err := s.approvals.Insert(am); err != nil {
			if governor.ErrorStatus(err) == http.StatusBadRequest {
				return nil, governor.NewErrorUser("", 0, err)
			}
			return nil, err
		}
	} else {
		code, err := s.approvals.RehashCode(am)
		if err != nil {
			return nil, err
		}
		if err := s.approvals.Insert(am); err != nil {
			if governor.ErrorStatus(err) == http.StatusBadRequest {
				return nil, governor.NewErrorUser("", 0, err)
			}
			return nil, err
		}
		if err := s.sendNewUserEmail(code, am); err != nil {
			return nil, err
		}
	}

	return &resUserUpdate{
		Userid:   m.Userid,
		Username: m.Username,
	}, nil
}

type (
	resApproval struct {
		Userid       string `json:"userid"`
		Username     string `json:"username"`
		Email        string `json:"email"`
		FirstName    string `json:"first_name"`
		LastName     string `json:"last_name"`
		CreationTime int64  `json:"creation_time"`
		Approved     bool   `json:"approved"`
		CodeTime     int64  `json:"code_time"`
	}

	resApprovals struct {
		Approvals []resApproval `json:"approvals"`
	}
)

func (s *service) GetUserApprovals(limit, offset int) (*resApprovals, error) {
	m, err := s.approvals.GetGroup(limit, offset)
	if err != nil {
		return nil, err
	}
	approvals := make([]resApproval, 0, len(m))
	for _, i := range m {
		approvals = append(approvals, resApproval{
			Userid:       i.Userid,
			Username:     i.Username,
			Email:        i.Email,
			FirstName:    i.FirstName,
			LastName:     i.LastName,
			CreationTime: i.CreationTime,
			Approved:     i.Approved,
			CodeTime:     i.CodeTime,
		})
	}
	return &resApprovals{
		Approvals: approvals,
	}, nil
}

func (s *service) ApproveUser(userid string) error {
	m, err := s.approvals.GetByID(userid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("", 0, err)
		}
		return err
	}
	code, err := s.approvals.RehashCode(m)
	if err != nil {
		return err
	}
	if err := s.approvals.Update(m); err != nil {
		return err
	}
	if err := s.sendNewUserEmail(code, m); err != nil {
		return err
	}
	return nil
}

func (s *service) DeleteUserApproval(userid string) error {
	m, err := s.approvals.GetByID(userid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("", 0, err)
		}
		return err
	}
	if err := s.approvals.Delete(m); err != nil {
		return err
	}
	return nil
}

func (s *service) sendNewUserEmail(code string, m *approvalmodel.Model) error {
	emdata := emailNewUser{
		Userid:    m.Userid,
		Key:       code,
		FirstName: m.FirstName,
		LastName:  m.LastName,
		Username:  m.Username,
	}
	if err := emdata.computeURL(s.emailurlbase, s.tplnewuser); err != nil {
		return err
	}
	if err := s.mailer.Send("", "", []string{m.Email}, newUserTemplate, emdata); err != nil {
		return governor.NewError("Failed to send account verification email", http.StatusInternalServerError, err)
	}
	return nil
}

// CommitUser takes a user from approvals and places it into the user db
func (s *service) CommitUser(userid string, key string) (*resUserUpdate, error) {
	am, err := s.approvals.GetByID(userid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, governor.NewErrorUser("", 0, err)
		}
		return nil, err
	}
	if !am.Approved {
		return nil, governor.NewErrorUser("Not approved", http.StatusBadRequest, nil)
	}
	if time.Now().Round(0).Unix() > am.CodeTime+s.confirmTime {
		return nil, governor.NewErrorUser("Code expired", http.StatusBadRequest, nil)
	}
	if ok, err := s.approvals.ValidateCode(key, am); err != nil {
		return nil, governor.NewError("Failed to verify key", http.StatusInternalServerError, err)
	} else if !ok {
		return nil, governor.NewErrorUser("Invalid key", http.StatusForbidden, nil)
	}
	m := s.approvals.ToUserModel(am)

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

	if err := s.users.Insert(m); err != nil {
		if governor.ErrorStatus(err) == http.StatusBadRequest {
			return nil, governor.NewErrorUser("", 0, err)
		}
		return nil, err
	}
	if err := s.roles.InsertRoles(m.Userid, rank.BaseUser()); err != nil {
		return nil, err
	}

	if err := s.queue.Publish(NewUserQueueID, b.Bytes()); err != nil {
		s.logger.Error("Failed to publish new user", map[string]string{
			"error":      err.Error(),
			"actiontype": "publishnewuser",
		})
	}

	if err := s.approvals.Delete(am); err != nil {
		s.logger.Error("Failed to clean up user approval", map[string]string{
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
	if roles, err := s.roles.IntersectRoles(userid, rank.Rank{"admin": struct{}{}}); err != nil {
		return err
	} else if roles.Has("admin") {
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

	if err := s.DeleteUserApikeys(userid); err != nil {
		return err
	}

	if err := s.KillAllSessions(userid); err != nil {
		return err
	}

	if err := s.roles.DeleteAllRoles(userid); err != nil {
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
