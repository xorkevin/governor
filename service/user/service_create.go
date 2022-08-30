package user

import (
	"bytes"
	"context"
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
	"xorkevin.dev/kerrors"
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
		return kerrors.WithMsg(err, "Failed executing new user url template")
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
func (s *service) CreateUser(ctx context.Context, ruser reqUserPost) (*resUserUpdate, error) {
	if _, err := s.users.GetByUsername(ctx, ruser.Username); err != nil {
		if !errors.Is(err, db.ErrNotFound{}) {
			return nil, kerrors.WithMsg(err, "Failed to get user")
		}
	} else {
		return nil, governor.ErrWithRes(nil, http.StatusBadRequest, "", "Username is already taken")
	}

	if _, err := s.users.GetByEmail(ctx, ruser.Email); err != nil {
		if !errors.Is(err, db.ErrNotFound{}) {
			return nil, kerrors.WithMsg(err, "Failed to get user")
		}
	} else {
		return nil, governor.ErrWithRes(nil, http.StatusBadRequest, "", "Email is already used by another account")
	}

	m, err := s.users.New(ruser.Username, ruser.Password, ruser.Email, ruser.FirstName, ruser.LastName)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to create new user request")
	}

	am := s.approvals.New(m)
	if s.userApproval {
		if err := s.approvals.Insert(ctx, am); err != nil {
			return nil, kerrors.WithMsg(err, "Failed to create new user request")
		}
	} else {
		code, err := s.approvals.RehashCode(am)
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to generate email verification code")
		}
		if err := s.approvals.Insert(ctx, am); err != nil {
			return nil, kerrors.WithMsg(err, "Failed to create new user request")
		}
		if err := s.sendNewUserEmail(ctx, code, am); err != nil {
			return nil, kerrors.WithMsg(err, "Failed to send new user email")
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

func (s *service) GetUserApprovals(ctx context.Context, limit, offset int) (*resApprovals, error) {
	m, err := s.approvals.GetGroup(ctx, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get user requests")
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

func (s *service) ApproveUser(ctx context.Context, userid string) error {
	m, err := s.approvals.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "User request not found")
		}
		return kerrors.WithMsg(err, "Failed to get user request")
	}
	code, err := s.approvals.RehashCode(m)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to generate new email verification code")
	}
	if err := s.approvals.Update(ctx, m); err != nil {
		return kerrors.WithMsg(err, "Failed to approve user")
	}
	if err := s.sendNewUserEmail(ctx, code, m); err != nil {
		return kerrors.WithMsg(err, "Failed to send account verification email")
	}
	return nil
}

func (s *service) DeleteUserApproval(ctx context.Context, userid string) error {
	m, err := s.approvals.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "User request not found")
		}
		return kerrors.WithMsg(err, "Failed to get user request")
	}
	if err := s.approvals.Delete(ctx, m); err != nil {
		return kerrors.WithMsg(err, "Failed to delete user request")
	}
	return nil
}

func (s *service) sendNewUserEmail(ctx context.Context, code string, m *approvalmodel.Model) error {
	emdata := emailNewUser{
		Userid:    m.Userid,
		Key:       code,
		FirstName: m.FirstName,
		LastName:  m.LastName,
		Username:  m.Username,
	}
	if err := emdata.computeURL(s.emailurl.base, s.emailurl.newuser); err != nil {
		return kerrors.WithMsg(err, "Failed to generate account verification email")
	}
	if err := s.mailer.SendTpl(ctx, "", mail.Addr{}, []mail.Addr{{Address: m.Email, Name: m.FirstName}}, mail.TplLocal(s.tplname.newuser), emdata, true); err != nil {
		return kerrors.WithMsg(err, "Failed to send account verification email")
	}
	return nil
}

// CommitUser takes a user from approvals and places it into the user db
func (s *service) CommitUser(ctx context.Context, userid string, key string) (*resUserUpdate, error) {
	am, err := s.approvals.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "User request not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get user request")
	}
	if !am.Approved {
		return nil, governor.ErrWithRes(nil, http.StatusBadRequest, "", "Not approved")
	}
	if time.Now().Round(0).Unix() > am.CodeTime+s.confirmTime {
		return nil, governor.ErrWithRes(nil, http.StatusBadRequest, "", "Code expired")
	}
	if ok, err := s.approvals.ValidateCode(key, am); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to verify key")
	} else if !ok {
		return nil, governor.ErrWithRes(nil, http.StatusUnauthorized, "", "Invalid key")
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
		return nil, kerrors.WithMsg(err, "Failed to encode user props to json")
	}

	if err := s.approvals.Delete(ctx, am); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to clean up user approval")
	}

	if err := s.users.Insert(ctx, m); err != nil {
		if errors.Is(err, db.ErrUnique{}) {
			return nil, governor.ErrWithRes(err, http.StatusBadRequest, "", "Username or email already in use by another account")
		}
		return nil, kerrors.WithMsg(err, "Failed to create user")
	}
	if err := s.roles.InsertRoles(ctx, m.Userid, rank.BaseUser()); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to create user roles")
	}

	// must make a best effort attempt to publish new user event
	if err := s.events.StreamPublish(context.Background(), s.opts.CreateChannel, b); err != nil {
		s.logger.Error("Failed to publish new user event", map[string]string{
			"error":      err.Error(),
			"actiontype": "user_publish_create",
		})
	}

	s.logger.Info("Created user", map[string]string{
		"userid":     m.Userid,
		"username":   m.Username,
		"actiontype": "user_create",
	})

	s.clearUserExists(userid)

	return &resUserUpdate{
		Userid:   m.Userid,
		Username: m.Username,
	}, nil
}

func (s *service) DeleteUser(ctx context.Context, userid string, username string, admin bool, password string) error {
	m, err := s.users.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "User not found")
		}
		return kerrors.WithMsg(err, "Failed to get user")
	}

	if m.Username != username {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Username does not match")
	}
	if roles, err := s.roles.IntersectRoles(ctx, userid, rank.Rank{"admin": struct{}{}}); err != nil {
		return kerrors.WithMsg(err, "Failed to get user roles")
	} else if roles.Has("admin") {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Not allowed to delete admin user")
	}
	if !admin {
		if ok, err := s.users.ValidatePass(password, m); err != nil {
			return kerrors.WithMsg(err, "Failed to validate password")
		} else if !ok {
			return governor.ErrWithRes(nil, http.StatusUnauthorized, "", "Incorrect password")
		}
	}

	j, err := json.Marshal(DeleteUserProps{
		Userid: m.Userid,
	})
	if err != nil {
		return kerrors.WithMsg(err, "Failed to encode user props to json")
	}
	if err := s.events.StreamPublish(ctx, s.opts.DeleteChannel, j); err != nil {
		return kerrors.WithMsg(err, "Failed to publish delete user event")
	}

	if err := s.resets.DeleteByUserid(ctx, userid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete user resets")
	}

	if err := s.KillAllSessions(ctx, userid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete user sessions")
	}

	if err := s.users.Delete(ctx, m); err != nil {
		return kerrors.WithMsg(err, "Failed to delete user roles")
	}

	s.clearUserExists(userid)
	return nil
}

func (s *service) clearUserExists(userid string) {
	// must make a best effort to clear user existence cache
	if err := s.kvusers.Del(context.Background(), userid); err != nil {
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
		return nil, kerrors.WithMsg(err, "Failed to decode new user props")
	}
	return m, nil
}

// DecodeDeleteUserProps unmarshals json encoded delete user props into a struct
func DecodeDeleteUserProps(msgdata []byte) (*DeleteUserProps, error) {
	m := &DeleteUserProps{}
	if err := json.Unmarshal(msgdata, m); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to decode delete user props")
	}
	return m, nil
}

// DecodeUpdateUserProps unmarshals json encoded update user props into a struct
func DecodeUpdateUserProps(msgdata []byte) (*UpdateUserProps, error) {
	m := &UpdateUserProps{}
	if err := json.Unmarshal(msgdata, m); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to decode update user props")
	}
	return m, nil
}
