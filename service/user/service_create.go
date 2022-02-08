package user

import (
	"bytes"
	"encoding/json"
	"errors"
	htmlTemplate "html/template"
	"net/http"
	"net/url"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/mail"
	approvalmodel "xorkevin.dev/governor/service/user/approval/model"
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
		return governor.ErrWithMsg(err, "Failed executing new user url template")
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
		if !errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.ErrWithMsg(err, "Failed to get user")
		}
	} else {
		return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Username is already taken",
		}))
	}

	if _, err := s.users.GetByEmail(ruser.Email); err != nil {
		if !errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.ErrWithMsg(err, "Failed to get user")
		}
	} else {
		return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Email is already used by another account",
		}))
	}

	m, err := s.users.New(ruser.Username, ruser.Password, ruser.Email, ruser.FirstName, ruser.LastName)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create new user request")
	}

	am := s.approvals.New(m)
	if s.userApproval {
		if err := s.approvals.Insert(am); err != nil {
			return nil, governor.ErrWithMsg(err, "Failed to create new user request")
		}
	} else {
		code, err := s.approvals.RehashCode(am)
		if err != nil {
			return nil, governor.ErrWithMsg(err, "Failed to generate email verification code")
		}
		if err := s.approvals.Insert(am); err != nil {
			return nil, governor.ErrWithMsg(err, "Failed to create new user request")
		}
		if err := s.sendNewUserEmail(code, am); err != nil {
			return nil, governor.ErrWithMsg(err, "Failed to send new user email")
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
		return nil, governor.ErrWithMsg(err, "Failed to get user requests")
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
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "User request not found",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get user request")
	}
	code, err := s.approvals.RehashCode(m)
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to generate new email verification code")
	}
	if err := s.approvals.Update(m); err != nil {
		return governor.ErrWithMsg(err, "Failed to approve user")
	}
	if err := s.sendNewUserEmail(code, m); err != nil {
		return governor.ErrWithMsg(err, "Failed to send account verification email")
	}
	return nil
}

func (s *service) DeleteUserApproval(userid string) error {
	m, err := s.approvals.GetByID(userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "User request not found",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get user request")
	}
	if err := s.approvals.Delete(m); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete user request")
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
		return governor.ErrWithMsg(err, "Failed to generate account verification email")
	}
	if err := s.mailer.Send("", mail.Addr{}, []mail.Addr{{Address: m.Email, Name: m.FirstName}}, mail.TplLocal(newUserTemplate), emdata, true); err != nil {
		return governor.ErrWithMsg(err, "Failed to send account verification email")
	}
	return nil
}

// CommitUser takes a user from approvals and places it into the user db
func (s *service) CommitUser(userid string, key string) (*resUserUpdate, error) {
	am, err := s.approvals.GetByID(userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "User request not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get user request")
	}
	if !am.Approved {
		return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Not approved",
		}))
	}
	if time.Now().Round(0).Unix() > am.CodeTime+s.confirmTime {
		return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Code expired",
		}))
	}
	if ok, err := s.approvals.ValidateCode(key, am); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to verify key")
	} else if !ok {
		return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusUnauthorized,
			Message: "Invalid key",
		}))
	}
	m := s.approvals.ToUserModel(am)

	b, err := json.Marshal(NewUserProps{
		Userid:       m.Userid,
		Username:     m.Username,
		Email:        m.Email,
		FirstName:    m.FirstName,
		LastName:     m.LastName,
		CreationTime: m.CreationTime,
	})
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to encode user props to json")
	}

	if err := s.users.Insert(m); err != nil {
		if errors.Is(err, db.ErrUnique{}) {
			if err := s.approvals.Delete(am); err != nil {
				s.logger.Error("Failed to clean up user approval", map[string]string{
					"error":      err.Error(),
					"actiontype": "commitusercleanup",
				})
			}
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusBadRequest,
				Message: "Username or email already in use by another account",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to create user")
	}
	if err := s.roles.InsertRoles(m.Userid, rank.BaseUser()); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create user roles")
	}

	if err := s.events.StreamPublish(s.opts.CreateChannel, b); err != nil {
		s.logger.Error("Failed to publish new user event", map[string]string{
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

	s.logger.Info("Created user", map[string]string{
		"userid":     m.Userid,
		"username":   m.Username,
		"actiontype": "commituser",
	})

	s.clearUserExists(userid)

	return &resUserUpdate{
		Userid:   m.Userid,
		Username: m.Username,
	}, nil
}

func (s *service) DeleteUser(userid string, username string, admin bool, password string) error {
	m, err := s.users.GetByID(userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "User not found",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get user")
	}

	if m.Username != username {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Username does not match",
		}))
	}
	if roles, err := s.roles.IntersectRoles(userid, rank.Rank{"admin": struct{}{}}); err != nil {
		return governor.ErrWithMsg(err, "Failed to get user roles")
	} else if roles.Has("admin") {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Not allowed to delete admin user",
		}))
	}
	if !admin {
		if ok, err := s.users.ValidatePass(password, m); err != nil {
			return governor.ErrWithMsg(err, "Failed to validate password")
		} else if !ok {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusBadRequest,
				Message: "Incorrect password",
			}))
		}
	}

	j, err := json.Marshal(DeleteUserProps{
		Userid: m.Userid,
	})
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to encode user props to json")
	}
	if err := s.events.StreamPublish(s.opts.DeleteChannel, j); err != nil {
		return governor.ErrWithMsg(err, "Failed to publish delete user event")
	}

	if err := s.resets.DeleteByUserid(userid); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete user resets")
	}

	if err := s.KillAllSessions(userid); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete user sessions")
	}

	if err := s.users.Delete(m); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete user roles")
	}

	s.clearUserExists(userid)
	return nil
}

func (s *service) clearUserExists(userid string) {
	if err := s.kvusers.Del(userid); err != nil {
		s.logger.Error("Failed to delete user exists in cache", map[string]string{
			"error":      err.Error(),
			"actiontype": "deluserexists",
		})
	}
}

// DecodeNewUserProps unmarshals json encoded new user props into a struct
func DecodeNewUserProps(msgdata []byte) (*NewUserProps, error) {
	m := &NewUserProps{}
	if err := json.Unmarshal(msgdata, m); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to decode new user props")
	}
	return m, nil
}

// DecodeDeleteUserProps unmarshals json encoded delete user props into a struct
func DecodeDeleteUserProps(msgdata []byte) (*DeleteUserProps, error) {
	m := &DeleteUserProps{}
	if err := json.Unmarshal(msgdata, m); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to decode delete user props")
	}
	return m, nil
}

// DecodeUpdateUserProps unmarshals json encoded update user props into a struct
func DecodeUpdateUserProps(msgdata []byte) (*UpdateUserProps, error) {
	m := &UpdateUserProps{}
	if err := json.Unmarshal(msgdata, m); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to decode update user props")
	}
	return m, nil
}
