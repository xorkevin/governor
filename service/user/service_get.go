package user

import (
	"errors"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/kvstore"
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
	ResUserInfo struct {
		Userid    string `json:"userid"`
		Username  string `json:"username"`
		Email     string `json:"email"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
	}

	ResUserInfoList struct {
		Users []ResUserInfo `json:"users"`
	}
)

// GetInfoAll gets and returns info for all users
func (s *service) GetInfoAll(amount int, offset int) (*ResUserInfoList, error) {
	infoSlice, err := s.users.GetGroup(amount, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get users")
	}

	info := make([]ResUserInfo, 0, len(infoSlice))
	for _, i := range infoSlice {
		info = append(info, ResUserInfo{
			Userid:    i.Userid,
			Username:  i.Username,
			Email:     i.Email,
			FirstName: i.FirstName,
			LastName:  i.LastName,
		})
	}

	return &ResUserInfoList{
		Users: info,
	}, nil
}

// GetInfoBulk gets and returns info for users
func (s *service) GetInfoBulk(userids []string) (*ResUserInfoList, error) {
	infoSlice, err := s.users.GetBulk(userids)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get users")
	}

	info := make([]ResUserInfo, 0, len(infoSlice))
	for _, i := range infoSlice {
		info = append(info, ResUserInfo{
			Userid:    i.Userid,
			Username:  i.Username,
			Email:     i.Email,
			FirstName: i.FirstName,
			LastName:  i.LastName,
		})
	}

	return &ResUserInfoList{
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

func (s *service) GetInfoUsernamePrefix(prefix string, limit int) (*resUserInfoListPublic, error) {
	if len(prefix) == 0 {
		return &resUserInfoListPublic{
			Users: []resUserInfoPublic{},
		}, nil
	}

	infoSlice, err := s.users.GetByUsernamePrefix(prefix, limit, 0)
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
		if !errors.Is(err, kvstore.ErrNotFound{}) {
			s.logger.Error("Failed to get user exists from cache", map[string]string{
				"error":      err.Error(),
				"actiontype": "getuserexists",
			})
		}
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

// CheckUsersExist is a fast check to determine if users exist
func (s *service) CheckUsersExist(userids []string) ([]string, error) {
	if len(userids) == 0 {
		return nil, nil
	}

	{
		m := map[string]struct{}{}
		l := make([]string, 0, len(userids))
		for _, i := range userids {
			if _, ok := m[i]; ok {
				continue
			}
			m[i] = struct{}{}
			l = append(l, i)
		}
		userids = l
	}

	res := make([]string, 0, len(userids))
	dneInCache := userids

	if multiget, err := s.kvusers.Multi(); err != nil {
		s.logger.Error("Failed to create kvstore multi", map[string]string{
			"error": err.Error(),
		})
	} else {
		results := make([]kvstore.Resulter, 0, len(userids))
		for _, i := range userids {
			results = append(results, multiget.Get(i))
		}
		if err := multiget.Exec(); err != nil {
			s.logger.Error("Failed to get userids from cache", map[string]string{
				"error":      err.Error(),
				"actiontype": "getusersexist",
			})
			goto end
		}
		dneInCache = make([]string, 0, len(userids))
		for n, i := range results {
			if v, err := i.Result(); err != nil {
				if !errors.Is(err, kvstore.ErrNotFound{}) {
					s.logger.Error("Failed to get user exists from cache", map[string]string{
						"error":      err.Error(),
						"actiontype": "getusersexistresult",
					})
				}
				dneInCache = append(dneInCache, userids[n])
			} else {
				if v == cacheValY {
					res = append(res, userids[n])
				}
			}
		}
	}

end:
	if len(dneInCache) == 0 {
		return res, nil
	}

	m, err := s.users.GetBulk(dneInCache)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get users")
	}

	userExists := map[string]struct{}{}
	for _, i := range m {
		res = append(res, i.Userid)
		userExists[i.Userid] = struct{}{}
	}

	multiset, err := s.kvusers.Multi()
	if err != nil {
		s.logger.Error("Failed to create kvstore multi", map[string]string{
			"error": err.Error(),
		})
		return res, nil
	}
	for _, i := range dneInCache {
		if _, ok := userExists[i]; ok {
			multiset.Set(i, cacheValY, s.userCacheTime)
		} else {
			multiset.Set(i, cacheValN, s.userCacheTime)
		}
	}
	if err := multiset.Exec(); err != nil {
		s.logger.Error("Failed to set users exist in cache", map[string]string{
			"error":      err.Error(),
			"actiontype": "setusersexist",
		})
	}

	return res, nil
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
