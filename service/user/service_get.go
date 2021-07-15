package user

import (
	"errors"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/user/model"
	"xorkevin.dev/governor/util/rank"
)

type (
	// ResUserGetPublic holds the public fields of a user
	ResUserGetPublic struct {
		Userid       string   `json:"userid"`
		Username     string   `json:"username"`
		Roles        []string `json:"roles"`
		FirstName    string   `json:"first_name"`
		LastName     string   `json:"last_name"`
		CreationTime int64    `json:"creation_time"`
	}
)

func getUserPublicFields(m *model.Model, roles []string) *ResUserGetPublic {
	return &ResUserGetPublic{
		Userid:       m.Userid,
		Username:     m.Username,
		Roles:        roles,
		FirstName:    m.FirstName,
		LastName:     m.LastName,
		CreationTime: m.CreationTime,
	}
}

type (
	// ResUserGet holds all the fields of a user
	ResUserGet struct {
		ResUserGetPublic
		Email      string `json:"email"`
		OTPEnabled bool   `json:"otp_enabled"`
	}
)

func getUserFields(m *model.Model, roles []string) *ResUserGet {
	return &ResUserGet{
		ResUserGetPublic: *getUserPublicFields(m, roles),
		Email:            m.Email,
		OTPEnabled:       m.OTPEnabled,
	}
}

func (s *service) getRoleSummary(userid string) (rank.Rank, error) {
	roles, err := s.roles.IntersectRoles(userid, s.rolesummary)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get user roles")
	}
	return roles, nil
}

// GetByIDPublic gets and returns the public fields of the user
func (s *service) GetByIDPublic(userid string) (*ResUserGetPublic, error) {
	m, err := s.users.GetByID(userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "User not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get user")
	}
	roles, err := s.getRoleSummary(userid)
	if err != nil {
		return nil, err
	}
	return getUserPublicFields(m, roles.ToSlice()), nil
}

// GetByID gets and returns all fields of the user
func (s *service) GetByID(userid string) (*ResUserGet, error) {
	m, err := s.users.GetByID(userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "User not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get user")
	}
	roles, err := s.getRoleSummary(userid)
	if err != nil {
		return nil, err
	}
	return getUserFields(m, roles.ToSlice()), nil
}

// GetByUsernamePublic gets and returns the public fields of the user
func (s *service) GetByUsernamePublic(username string) (*ResUserGetPublic, error) {
	m, err := s.users.GetByUsername(username)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "User not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get user")
	}
	roles, err := s.getRoleSummary(m.Userid)
	if err != nil {
		return nil, err
	}
	return getUserPublicFields(m, roles.ToSlice()), nil
}

// GetByUsername gets and returns all fields of the user
func (s *service) GetByUsername(username string) (*ResUserGet, error) {
	m, err := s.users.GetByUsername(username)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "User not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get user")
	}
	roles, err := s.getRoleSummary(m.Userid)
	if err != nil {
		return nil, err
	}
	return getUserFields(m, roles.ToSlice()), nil
}

// GetByEmail gets and returns all fields of the user
func (s *service) GetByEmail(email string) (*ResUserGet, error) {
	m, err := s.users.GetByEmail(email)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "User not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get user")
	}
	roles, err := s.getRoleSummary(m.Userid)
	if err != nil {
		return nil, err
	}
	return getUserFields(m, roles.ToSlice()), nil
}

type (
	resUserRoles struct {
		Roles []string `json:"roles"`
	}
)

// GetUserRoles returns a list of user roles
func (s *service) GetUserRoles(userid string, prefix string, amount, offset int) (*resUserRoles, error) {
	roles, err := s.roles.GetRoles(userid, prefix, amount, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get user roles")
	}
	return &resUserRoles{
		Roles: roles.ToSlice(),
	}, nil
}

// GetUserRolesIntersect returns the intersected roles of a user
func (s *service) GetUserRolesIntersect(userid string, roleset rank.Rank) (*resUserRoles, error) {
	roles, err := s.roles.IntersectRoles(userid, roleset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get user roles")
	}
	return &resUserRoles{
		Roles: roles.ToSlice(),
	}, nil
}

type (
	resUserInfo struct {
		Userid   string `json:"userid"`
		Username string `json:"username"`
		Email    string `json:"email"`
	}

	resUserInfoList struct {
		Users []resUserInfo `json:"users"`
	}
)

// GetInfoAll gets and returns info for all users
func (s *service) GetInfoAll(amount int, offset int) (*resUserInfoList, error) {
	infoSlice, err := s.users.GetGroup(amount, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get users")
	}

	info := make([]resUserInfo, 0, len(infoSlice))
	for _, i := range infoSlice {
		info = append(info, resUserInfo{
			Userid:   i.Userid,
			Username: i.Username,
			Email:    i.Email,
		})
	}

	return &resUserInfoList{
		Users: info,
	}, nil
}

type (
	resUserInfoPublic struct {
		Userid    string `json:"userid"`
		Username  string `json:"username"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
	}

	resUserInfoListPublic struct {
		Users []resUserInfoPublic `json:"users"`
	}
)

// GetInfoBulkPublic gets and returns public info for users
func (s *service) GetInfoBulkPublic(userids []string) (*resUserInfoListPublic, error) {
	infoSlice, err := s.users.GetBulk(userids)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get users")
	}

	info := make([]resUserInfoPublic, 0, len(infoSlice))
	for _, i := range infoSlice {
		info = append(info, resUserInfoPublic{
			Userid:    i.Userid,
			Username:  i.Username,
			FirstName: i.FirstName,
			LastName:  i.LastName,
		})
	}

	return &resUserInfoListPublic{
		Users: info,
	}, nil
}

type (
	resUserList struct {
		Users []string `json:"users"`
	}
)

// GetIDsByRole retrieves a list of user ids by role
func (s *service) GetIDsByRole(role string, amount int, offset int) (*resUserList, error) {
	userids, err := s.roles.GetByRole(role, amount, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get users")
	}
	return &resUserList{
		Users: userids,
	}, nil
}

const (
	cacheValY = "y"
	cacheValN = "n"
)

// CheckUserExists is a fast check to determine if a user exists
func (s *service) CheckUserExists(userid string) (bool, error) {
	if v, err := s.kvusers.Get(userid); err != nil {
		s.logger.Error("Failed to get user exists from cache", map[string]string{
			"error":      err.Error(),
			"actiontype": "getuserexists",
		})
	} else {
		if v == cacheValY {
			return true, nil
		}
		return false, nil
	}

	exists := true
	_, err := s.users.GetByID(userid)
	if err != nil {
		if !errors.Is(err, db.ErrNotFound{}) {
			return false, governor.ErrWithMsg(err, "Failed to get user")
		}
		exists = false
	}

	v := cacheValN
	if exists {
		v = cacheValY
	}
	if err := s.kvusers.Set(userid, v, s.userCacheTime); err != nil {
		s.logger.Error("Failed to set user exists in cache", map[string]string{
			"error":      err.Error(),
			"actiontype": "setuserexists",
		})
	}

	return exists, nil
}

// GetUseridForLogin gets a userid for login
func (s *service) GetUseridForLogin(useroremail string) (string, error) {
	if isEmail(useroremail) {
		m, err := s.users.GetByEmail(useroremail)
		if err != nil {
			if errors.Is(err, db.ErrNotFound{}) {
				return "", governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
					Status:  http.StatusUnauthorized,
					Message: "Invalid username or password",
				}), governor.ErrOptInner(err))
			}
			return "", governor.ErrWithMsg(err, "Failed to get user")
		}
		return m.Userid, nil
	}
	m, err := s.users.GetByUsername(useroremail)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return "", governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusUnauthorized,
				Message: "Invalid username or password",
			}), governor.ErrOptInner(err))
		}
		return "", governor.ErrWithMsg(err, "Failed to get user")
	}
	return m.Userid, nil
}
