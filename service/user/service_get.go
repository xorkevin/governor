package user

import (
	"context"
	"errors"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/authzacl"
	"xorkevin.dev/governor/service/dbsql"
	"xorkevin.dev/governor/service/gate"
	"xorkevin.dev/governor/service/user/usermodel"
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
		Userid       string `json:"userid"`
		Username     string `json:"username"`
		FirstName    string `json:"first_name"`
		LastName     string `json:"last_name"`
		CreationTime int64  `json:"creation_time"`
	}
)

func getUserPublicFields(m *usermodel.Model) *ResUserGetPublic {
	return &ResUserGetPublic{
		Userid:       m.Userid,
		Username:     m.Username,
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

func getUserFields(m *usermodel.Model) *ResUserGet {
	return &ResUserGet{
		ResUserGetPublic: *getUserPublicFields(m),
		Email:            m.Email,
		OTPEnabled:       m.OTPEnabled,
	}
}

func (s *Service) getByIDPublic(ctx context.Context, userid string) (*ResUserGetPublic, error) {
	m, err := s.users.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, dbsql.ErrNotFound) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "User not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get user")
	}
	return getUserPublicFields(m), nil
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
	return getUserFields(m), nil
}

func (s *Service) getByUsernamePublic(ctx context.Context, username string) (*ResUserGetPublic, error) {
	m, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, dbsql.ErrNotFound) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "User not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get user")
	}
	return getUserPublicFields(m), nil
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
	return getUserFields(m), nil
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
	return getUserFields(m), nil
}

type (
	resUserRoles struct {
		Roles []string `json:"roles"`
	}
)

func (s *Service) getUserRoles(ctx context.Context, userid string, prefix string, amount int) (*resUserRoles, error) {
	roles, err := s.acl.ReadBySubObjPred(ctx, authzacl.Sub{NS: gate.NSUser, Key: userid}, gate.NSRole, gate.RelIn, prefix, amount)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get user roles")
	}
	return &resUserRoles{
		Roles: roles,
	}, nil
}

func (s *Service) getUserMods(ctx context.Context, userid string, prefix string, amount int) (*resUserRoles, error) {
	roles, err := s.acl.ReadBySubObjPred(ctx, authzacl.Sub{NS: gate.NSUser, Key: userid}, gate.NSRole, gate.RelMod, prefix, amount)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get user roles")
	}
	return &resUserRoles{
		Roles: roles,
	}, nil
}

func (s *Service) getUserRolesIntersect(ctx context.Context, userid string, roles []string) (*resUserRoles, error) {
	res := make([]string, 0, len(roles))
	for _, i := range roles {
		ok, err := gate.CheckRole(ctx, s.acl, userid, i)
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to get user roles")
		}
		if ok {
			res = append(res, i)
		}
	}
	return &resUserRoles{
		Roles: res,
	}, nil
}

func (s *Service) getUserModsIntersect(ctx context.Context, userid string, roles []string) (*resUserRoles, error) {
	res := make([]string, 0, len(roles))
	for _, i := range roles {
		ok, err := gate.CheckMod(ctx, s.acl, userid, i)
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to get user roles")
		}
		if ok {
			res = append(res, i)
		}
	}
	return &resUserRoles{
		Roles: res,
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

// GetInfoMany implements [Users] and gets and returns info for users
func (s *Service) GetInfoMany(ctx context.Context, userids []string) (*ResUserInfoList, error) {
	infoSlice, err := s.users.GetMany(ctx, userids)
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

func (s *Service) getInfoManyPublic(ctx context.Context, userids []string) (*resUserInfoListPublic, error) {
	infoSlice, err := s.users.GetMany(ctx, userids)
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
		Userids []string `json:"userids"`
	}
)

func (s *Service) getIDsByRole(ctx context.Context, role string, afterUserid string, amount int) (*resUserList, error) {
	subs, err := s.acl.Read(ctx, authzacl.ObjRel{NS: gate.NSRole, Key: role, Pred: gate.RelIn}, &authzacl.Sub{NS: gate.NSUser, Key: afterUserid}, amount)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get users")
	}
	userids := make([]string, 0, len(subs))
	for _, i := range subs {
		if i.NS == gate.NSUser {
			userids = append(userids, i.Key)
		}
	}
	return &resUserList{
		Userids: userids,
	}, nil
}

func (s *Service) getIDsByMod(ctx context.Context, role string, afterUserid string, amount int) (*resUserList, error) {
	subs, err := s.acl.Read(ctx, authzacl.ObjRel{NS: gate.NSRole, Key: role, Pred: gate.RelMod}, &authzacl.Sub{NS: gate.NSUser, Key: afterUserid}, amount)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get users")
	}
	userids := make([]string, 0, len(subs))
	for _, i := range subs {
		if i.NS == gate.NSUser {
			userids = append(userids, i.Key)
		}
	}
	return &resUserList{
		Userids: userids,
	}, nil
}

// CheckUserExists implements [Users] and is a fast check to determine if a user exists
func (s *Service) CheckUserExists(ctx context.Context, userid string) (bool, error) {
	exists, err := s.users.Exists(ctx, userid)
	if err != nil {
		return false, kerrors.WithMsg(err, "Failed to get user")
	}
	return exists, nil
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
