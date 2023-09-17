package user

import (
	"context"
	"errors"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/dbsql"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/user/usermodel"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/kerrors"
)

// ErrNotFound is returned when the user does not exist
var ErrNotFound errNotFound

type (
	errNotFound struct{}
)

func (e errNotFound) Error() string {
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

func getUserPublicFields(m *usermodel.Model, roles []string) *ResUserGetPublic {
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

func getUserFields(m *usermodel.Model, roles []string) *ResUserGet {
	return &ResUserGet{
		ResUserGetPublic: *getUserPublicFields(m, roles),
		Email:            m.Email,
		OTPEnabled:       m.OTPEnabled,
	}
}

func (s *Service) getRoleSummary(ctx context.Context, userid string) (rank.Rank, error) {
	roles, err := s.roles.IntersectRoles(ctx, userid, s.rolesummary)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get user roles")
	}
	return roles, nil
}

func (s *Service) getByIDPublic(ctx context.Context, userid string) (*ResUserGetPublic, error) {
	m, err := s.users.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, dbsql.ErrNotFound) {
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

// GetByID implements [Users] and gets and returns all fields of the user
func (s *Service) GetByID(ctx context.Context, userid string) (*ResUserGet, error) {
	m, err := s.users.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, dbsql.ErrNotFound) {
			return nil, governor.ErrWithRes(kerrors.WithKind(err, ErrNotFound, "User not found"), http.StatusNotFound, "", "User not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get user")
	}
	roles, err := s.getRoleSummary(ctx, userid)
	if err != nil {
		return nil, err
	}
	return getUserFields(m, roles.ToSlice()), nil
}

func (s *Service) getByUsernamePublic(ctx context.Context, username string) (*ResUserGetPublic, error) {
	m, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, dbsql.ErrNotFound) {
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

// GetByUsername implements [Users] and gets and returns all fields of the user
func (s *Service) GetByUsername(ctx context.Context, username string) (*ResUserGet, error) {
	m, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, dbsql.ErrNotFound) {
			return nil, governor.ErrWithRes(kerrors.WithKind(err, ErrNotFound, "User not found"), http.StatusNotFound, "", "User not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get user")
	}
	roles, err := s.getRoleSummary(ctx, m.Userid)
	if err != nil {
		return nil, err
	}
	return getUserFields(m, roles.ToSlice()), nil
}

// GetByEmail implements [Users] and gets and returns all fields of the user
func (s *Service) GetByEmail(ctx context.Context, email string) (*ResUserGet, error) {
	m, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, dbsql.ErrNotFound) {
			return nil, governor.ErrWithRes(kerrors.WithKind(err, ErrNotFound, "User not found"), http.StatusNotFound, "", "User not found")
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

func (s *Service) getUserRoles(ctx context.Context, userid string, prefix string, amount, offset int) (*resUserRoles, error) {
	roles, err := s.roles.GetRoles(ctx, userid, prefix, amount, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get user roles")
	}
	return &resUserRoles{
		Roles: roles.ToSlice(),
	}, nil
}

func (s *Service) getUserRolesIntersect(ctx context.Context, userid string, roleset rank.Rank) (*resUserRoles, error) {
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

func (s *Service) getInfoAll(ctx context.Context, amount int, offset int) (*ResUserInfoList, error) {
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

// GetInfoBulk implements [Users] and gets and returns info for users
func (s *Service) GetInfoBulk(ctx context.Context, userids []string) (*ResUserInfoList, error) {
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

func (s *Service) getInfoBulkPublic(ctx context.Context, userids []string) (*resUserInfoListPublic, error) {
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

func (s *Service) getInfoUsernamePrefix(ctx context.Context, prefix string, limit int) (*resUserInfoListPublic, error) {
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

func (s *Service) getIDsByRole(ctx context.Context, role string, amount int, offset int) (*resUserList, error) {
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

// CheckUserExists implements [Users] and is a fast check to determine if a user exists
func (s *Service) CheckUserExists(ctx context.Context, userid string) (bool, error) {
	if v, err := s.kvusers.Get(ctx, userid); err != nil {
		if !errors.Is(err, kvstore.ErrNotFound) {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get user exists from cache"))
		}
	} else {
		if v == cacheValY {
			return true, nil
		}
		return false, nil
	}

	exists := true
	if _, err := s.users.GetByID(ctx, userid); err != nil {
		if !errors.Is(err, dbsql.ErrNotFound) {
			return false, kerrors.WithMsg(err, "Failed to get user")
		}
		exists = false
	}

	v := cacheValN
	if exists {
		v = cacheValY
	}
	if err := s.kvusers.Set(ctx, userid, v, s.authsettings.userCacheDuration); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to set user exists in cache"))
	}

	return exists, nil
}

// CheckUsersExist implements [Users] and is a fast check to determine if users exist
func (s *Service) CheckUsersExist(ctx context.Context, userids []string) ([]string, error) {
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
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to create kvstore multi"))
	} else {
		results := make([]kvstore.Resulter, 0, len(userids))
		for _, i := range userids {
			results = append(results, multiget.Get(ctx, i))
		}
		if err := multiget.Exec(ctx); err != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get userids from cache"))
			goto end
		}
		dneInCache = make([]string, 0, len(userids))
		for n, i := range results {
			if v, err := i.Result(); err != nil {
				if !errors.Is(err, kvstore.ErrNotFound) {
					s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get user exists from cache"))
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
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to create kvstore multi"))
		return res, nil
	}
	for _, i := range dneInCache {
		if _, ok := userExists[i]; ok {
			multiset.Set(ctx, i, cacheValY, s.authsettings.userCacheDuration)
		} else {
			multiset.Set(ctx, i, cacheValN, s.authsettings.userCacheDuration)
		}
	}
	if err := multiset.Exec(ctx); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to set users exist in cache"))
	}

	return res, nil
}

func (s *Service) getUseridForLogin(ctx context.Context, username string, email string) (string, error) {
	if email != "" {
		m, err := s.users.GetByEmail(ctx, email)
		if err != nil {
			if errors.Is(err, dbsql.ErrNotFound) {
				return "", governor.ErrWithRes(err, http.StatusUnauthorized, "", "Invalid username or password")
			}
			return "", kerrors.WithMsg(err, "Failed to get user")
		}
		return m.Userid, nil
	}
	m, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, dbsql.ErrNotFound) {
			return "", governor.ErrWithRes(err, http.StatusUnauthorized, "", "Invalid username or password")
		}
		return "", kerrors.WithMsg(err, "Failed to get user")
	}
	return m.Userid, nil
}
