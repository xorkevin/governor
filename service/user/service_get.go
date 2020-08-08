package user

import (
	"net/http"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/model"
	"xorkevin.dev/governor/util/rank"
)

type (
	// ResUserGetPublic holds the public fields of a user
	ResUserGetPublic struct {
		Userid       string   `json:"userid"`
		Username     string   `json:"username"`
		AuthTags     []string `json:"auth_tags"`
		FirstName    string   `json:"first_name"`
		LastName     string   `json:"last_name"`
		CreationTime int64    `json:"creation_time"`
	}
)

func getUserPublicFields(m *usermodel.Model, roles []string) *ResUserGetPublic {
	return &ResUserGetPublic{
		Userid:       m.Userid,
		Username:     m.Username,
		AuthTags:     roles,
		FirstName:    m.FirstName,
		LastName:     m.LastName,
		CreationTime: m.CreationTime,
	}
}

type (
	// ResUserGet holds all the fields of a user
	ResUserGet struct {
		ResUserGetPublic
		Email string `json:"email"`
	}
)

func getUserFields(m *usermodel.Model, roles []string) *ResUserGet {
	return &ResUserGet{
		ResUserGetPublic: *getUserPublicFields(m, roles),
		Email:            m.Email,
	}
}

// GetByIDPublic gets and returns the public fields of the user
func (s *service) GetByIDPublic(userid string) (*ResUserGetPublic, error) {
	m, err := s.users.GetByID(userid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, governor.NewErrorUser("", 0, err)
		}
		return nil, err
	}
	roles, err := s.roles.GetRoleSummary(userid)
	if err != nil {
		return nil, err
	}
	return getUserPublicFields(m, roles.ToSlice()), nil
}

// GetByID gets and returns all fields of the user
func (s *service) GetByID(userid string) (*ResUserGet, error) {
	m, err := s.users.GetByID(userid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, governor.NewErrorUser("", 0, err)
		}
		return nil, err
	}
	roles, err := s.roles.GetRoleSummary(userid)
	if err != nil {
		return nil, err
	}
	return getUserFields(m, roles.ToSlice()), nil
}

// GetByUsernamePublic gets and returns the public fields of the user
func (s *service) GetByUsernamePublic(username string) (*ResUserGetPublic, error) {
	m, err := s.users.GetByUsername(username)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, governor.NewErrorUser("", 0, err)
		}
		return nil, err
	}
	roles, err := s.roles.GetRoleSummary(m.Userid)
	if err != nil {
		return nil, err
	}
	return getUserPublicFields(m, roles.ToSlice()), nil
}

// GetByUsername gets and returns all fields of the user
func (s *service) GetByUsername(username string) (*ResUserGet, error) {
	m, err := s.users.GetByUsername(username)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, governor.NewErrorUser("", 0, err)
		}
		return nil, err
	}
	roles, err := s.roles.GetRoleSummary(m.Userid)
	if err != nil {
		return nil, err
	}
	return getUserFields(m, roles.ToSlice()), nil
}

// GetByEmail gets and returns all fields of the user
func (s *service) GetByEmail(email string) (*ResUserGet, error) {
	m, err := s.users.GetByEmail(email)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, governor.NewErrorUser("", 0, err)
		}
		return nil, err
	}
	roles, err := s.roles.GetRoleSummary(m.Userid)
	if err != nil {
		return nil, err
	}
	return getUserFields(m, roles.ToSlice()), nil
}

type (
	resUserRoles struct {
		Roles []string `json:"auth_tags"`
	}
)

// GetUserRoles returns a list of user roles
func (s *service) GetUserRoles(userid string, amount, offset int) (*resUserRoles, error) {
	roles, err := s.roles.GetRoles(userid, amount, offset)
	if err != nil {
		return nil, err
	}
	return &resUserRoles{
		Roles: roles.ToSlice(),
	}, nil
}

// GetUserRolesIntersect returns the intersected roles of a user
func (s *service) GetUserRolesIntersect(userid string, roleset rank.Rank) (*resUserRoles, error) {
	roles, err := s.roles.IntersectRoles(userid, roleset)
	if err != nil {
		return nil, err
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
		return nil, err
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
		return nil, err
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
		return nil, err
	}
	return &resUserList{
		Users: userids,
	}, nil
}
