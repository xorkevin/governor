package user

import (
	"context"
	"errors"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/user/model"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/kerrors"
)

type (
	ErrNotFound struct{}
)

func (e ErrNotFound) Error() string {
	return "User not found"
}

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

func (s *service) getRoleSummary(ctx context.Context, userid string) (rank.Rank, error) {
	roles, err := s.roles.IntersectRoles(ctx, userid, s.rolesummary)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get user roles")
	}
	return roles, nil
}

// GetByIDPublic gets and returns the public fields of the user
func (s *service) GetByIDPublic(ctx context.Context, userid string) (*ResUserGetPublic, error) {
	m, err := s.users.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "User not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get user")
	}
	roles, err := s.getRoleSummary(ctx, userid)
	if err != nil {
		return nil, err
	}
	return getUserPublicFields(m, roles.ToSlice()), nil
}

// GetByID gets and returns all fields of the user
func (s *service) GetByID(ctx context.Context, userid string) (*ResUserGet, error) {
	m, err := s.users.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.ErrWithRes(kerrors.WithKind(err, ErrNotFound{}, "User not found"), http.StatusNotFound, "", "User not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get user")
	}
	roles, err := s.getRoleSummary(ctx, userid)
	if err != nil {
		return nil, err
	}
	return getUserFields(m, roles.ToSlice()), nil
}

// GetByUsernamePublic gets and returns the public fields of the user
func (s *service) GetByUsernamePublic(ctx context.Context, username string) (*ResUserGetPublic, error) {
	m, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "User not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get user")
	}
	roles, err := s.getRoleSummary(ctx, m.Userid)
	if err != nil {
		return nil, err
	}
	return getUserPublicFields(m, roles.ToSlice()), nil
}

// GetByUsername gets and returns all fields of the user
func (s *service) GetByUsername(ctx context.Context, username string) (*ResUserGet, error) {
	m, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.ErrWithRes(kerrors.WithKind(err, ErrNotFound{}, "User not found"), http.StatusNotFound, "", "User not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get user")
	}
	roles, err := s.getRoleSummary(ctx, m.Userid)
	if err != nil {
		return nil, err
	}
	return getUserFields(m, roles.ToSlice()), nil
}

// GetByEmail gets and returns all fields of the user
func (s *service) GetByEmail(ctx context.Context, email string) (*ResUserGet, error) {
	m, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.ErrWithRes(kerrors.WithKind(err, ErrNotFound{}, "User not found"), http.StatusNotFound, "", "User not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get user")
	}
	roles, err := s.getRoleSummary(ctx, m.Userid)
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
func (s *service) GetUserRoles(ctx context.Context, userid string, prefix string, amount, offset int) (*resUserRoles, error) {
	roles, err := s.roles.GetRoles(ctx, userid, prefix, amount, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get user roles")
	}
	return &resUserRoles{
		Roles: roles.ToSlice(),
	}, nil
}

// GetUserRolesIntersect returns the intersected roles of a user
func (s *service) GetUserRolesIntersect(ctx context.Context, userid string, roleset rank.Rank) (*resUserRoles, error) {
	roles, err := s.roles.IntersectRoles(ctx, userid, roleset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get user roles")
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
func (s *service) GetInfoAll(ctx context.Context, amount int, offset int) (*ResUserInfoList, error) {
	infoSlice, err := s.users.GetGroup(ctx, amount, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get users")
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
func (s *service) GetInfoBulk(ctx context.Context, userids []string) (*ResUserInfoList, error) {
	infoSlice, err := s.users.GetBulk(ctx, userids)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get users")
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
func (s *service) GetInfoBulkPublic(ctx context.Context, userids []string) (*resUserInfoListPublic, error) {
	infoSlice, err := s.users.GetBulk(ctx, userids)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get users")
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

func (s *service) GetInfoUsernamePrefix(ctx context.Context, prefix string, limit int) (*resUserInfoListPublic, error) {
	if len(prefix) == 0 {
		return &resUserInfoListPublic{
			Users: []resUserInfoPublic{},
		}, nil
	}

	infoSlice, err := s.users.GetByUsernamePrefix(ctx, prefix, limit, 0)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get users")
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
func (s *service) GetIDsByRole(ctx context.Context, role string, amount int, offset int) (*resUserList, error) {
	userids, err := s.roles.GetByRole(ctx, role, amount, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get users")
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
func (s *service) CheckUserExists(ctx context.Context, userid string) (bool, error) {
	if v, err := s.kvusers.Get(ctx, userid); err != nil {
		if !errors.Is(err, kvstore.ErrNotFound{}) {
			s.logger.Error("Failed to get user exists from cache", map[string]string{
				"error":      err.Error(),
				"actiontype": "user_get_cache_exists",
			})
		}
	} else {
		if v == cacheValY {
			return true, nil
		}
		return false, nil
	}

	exists := true
	if _, err := s.users.GetByID(ctx, userid); err != nil {
		if !errors.Is(err, db.ErrNotFound{}) {
			return false, kerrors.WithMsg(err, "Failed to get user")
		}
		exists = false
	}

	v := cacheValN
	if exists {
		v = cacheValY
	}
	if err := s.kvusers.Set(ctx, userid, v, s.userCacheTime); err != nil {
		s.logger.Error("Failed to set user exists in cache", map[string]string{
			"error":      err.Error(),
			"actiontype": "user_set_cache_exists",
		})
	}

	return exists, nil
}

// CheckUsersExist is a fast check to determine if users exist
func (s *service) CheckUsersExist(ctx context.Context, userids []string) ([]string, error) {
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

	if multiget, err := s.kvusers.Multi(ctx); err != nil {
		s.logger.Error("Failed to create kvstore multi", map[string]string{
			"error":      err.Error(),
			"actiontype": "user_get_cache_exists",
		})
	} else {
		results := make([]kvstore.Resulter, 0, len(userids))
		for _, i := range userids {
			results = append(results, multiget.Get(ctx, i))
		}
		if err := multiget.Exec(ctx); err != nil {
			s.logger.Error("Failed to get userids from cache", map[string]string{
				"error":      err.Error(),
				"actiontype": "user_get_cache_exists",
			})
			goto end
		}
		dneInCache = make([]string, 0, len(userids))
		for n, i := range results {
			if v, err := i.Result(); err != nil {
				if !errors.Is(err, kvstore.ErrNotFound{}) {
					s.logger.Error("Failed to get user exists from cache", map[string]string{
						"error":      err.Error(),
						"actiontype": "user_get_cache_exists_result",
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

	m, err := s.users.GetBulk(ctx, dneInCache)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get users")
	}

	userExists := map[string]struct{}{}
	for _, i := range m {
		res = append(res, i.Userid)
		userExists[i.Userid] = struct{}{}
	}

	multiset, err := s.kvusers.Multi(ctx)
	if err != nil {
		s.logger.Error("Failed to create kvstore multi", map[string]string{
			"error":      err.Error(),
			"actiontype": "user_set_cache_exists",
		})
		return res, nil
	}
	for _, i := range dneInCache {
		if _, ok := userExists[i]; ok {
			multiset.Set(ctx, i, cacheValY, s.userCacheTime)
		} else {
			multiset.Set(ctx, i, cacheValN, s.userCacheTime)
		}
	}
	if err := multiset.Exec(ctx); err != nil {
		s.logger.Error("Failed to set users exist in cache", map[string]string{
			"error":      err.Error(),
			"actiontype": "user_set_cache_exists",
		})
	}

	return res, nil
}

// GetUseridForLogin gets a userid for login
func (s *service) GetUseridForLogin(ctx context.Context, useroremail string) (string, error) {
	if isEmail(useroremail) {
		m, err := s.users.GetByEmail(ctx, useroremail)
		if err != nil {
			if errors.Is(err, db.ErrNotFound{}) {
				return "", governor.ErrWithRes(err, http.StatusUnauthorized, "", "Invalid username or password")
			}
			return "", kerrors.WithMsg(err, "Failed to get user")
		}
		return m.Userid, nil
	}
	m, err := s.users.GetByUsername(ctx, useroremail)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return "", governor.ErrWithRes(err, http.StatusUnauthorized, "", "Invalid username or password")
		}
		return "", kerrors.WithMsg(err, "Failed to get user")
	}
	return m.Userid, nil
}
