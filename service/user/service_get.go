package user

import (
	"bytes"
	"encoding/gob"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/model"
	"github.com/hackform/governor/service/user/role/model"
	"github.com/hackform/governor/service/user/session"
	"net/http"
	"sort"
)

// GetUser gets and returns a user with the specified id
func (u *userService) GetUser(userid string) (*usermodel.Model, *governor.Error) {
	m, err := usermodel.GetByIDB64(u.db.DB(), userid)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return nil, err
	}
	return m, nil
}

type (
	resUserGetPublic struct {
		Userid       string `json:"userid"`
		Username     string `json:"username"`
		Tags         string `json:"auth_tags"`
		FirstName    string `json:"first_name"`
		LastName     string `json:"last_name"`
		CreationTime int64  `json:"creation_time"`
	}
)

func getUserPublicFields(m *usermodel.Model) *resUserGetPublic {
	userid, _ := m.IDBase64()
	return &resUserGetPublic{
		Userid:       userid,
		Username:     m.Username,
		Tags:         m.Tags,
		FirstName:    m.FirstName,
		LastName:     m.LastName,
		CreationTime: m.CreationTime,
	}
}

type (
	resUserGet struct {
		resUserGetPublic
		Email string `json:"email"`
	}
)

func getUserFields(m *usermodel.Model) *resUserGet {
	return &resUserGet{
		resUserGetPublic: *getUserPublicFields(m),
		Email:            m.Email,
	}
}

// GetByIdPublic gets and returns the public fields of the user
func (u *userService) GetByIdPublic(userid string) (*resUserGetPublic, *governor.Error) {
	m, err := usermodel.GetByIDB64(u.db.DB(), userid)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return nil, err
	}
	return getUserPublicFields(m), nil
}

// GetById gets and returns all fields of the user
func (u *userService) GetById(userid string) (*resUserGet, *governor.Error) {
	m, err := usermodel.GetByIDB64(u.db.DB(), userid)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return nil, err
	}
	return getUserFields(m), nil
}

// GetByUsernamePublic gets and returns the public fields of the user
func (u *userService) GetByUsernamePublic(username string) (*resUserGetPublic, *governor.Error) {
	m, err := usermodel.GetByUsername(u.db.DB(), username)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return nil, err
	}
	return getUserPublicFields(m), nil
}

// GetByUsername gets and returns all fields of the user
func (u *userService) GetByUsername(username string) (*resUserGet, *governor.Error) {
	m, err := usermodel.GetByUsername(u.db.DB(), username)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return nil, err
	}
	return getUserFields(m), nil
}

type (
	resUserInfo struct {
		Userid string `json:"userid"`
		Email  string `json:"email"`
	}

	userInfoSlice []resUserInfo

	resUserInfoList struct {
		Users userInfoSlice `json:"users"`
	}
)

// GetInfoAll gets and returns info for all users
func (u *userService) GetInfoAll(amount int, offset int) (*resUserInfoList, *governor.Error) {
	infoSlice, err := usermodel.GetGroup(u.db.DB(), amount, offset)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return nil, err
	}

	info := make(userInfoSlice, 0, len(infoSlice))
	for _, i := range infoSlice {
		useruid, _ := i.IDBase64()

		info = append(info, resUserInfo{
			Userid: useruid,
			Email:  i.Email,
		})
	}

	return &resUserInfoList{
		Users: info,
	}, nil
}

type (
	resUserList struct {
		Users []string `json:"users"`
	}
)

// GetIDsByRole retrieves a list of user ids by role
func (u *userService) GetIDsByRole(role string, amount int, offset int) (*resUserList, *governor.Error) {
	userids, err := rolemodel.GetByRole(u.db.DB(), role, amount, offset)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return nil, err
	}
	return &resUserList{
		Users: userids,
	}, nil
}

type (
	resUserGetSessions struct {
		Sessions []session.Session `json:"active_sessions"`
	}
)

// GetSessions retrieves a list of user sessions
func (u *userService) GetSessions(userid string) (*resUserGetSessions, *governor.Error) {
	ch := u.cache.Cache()

	s := session.Session{
		Userid: userid,
	}

	var sarr session.Slice
	if sgobs, err := ch.HGetAll(s.UserKey()).Result(); err == nil {
		sarr = make(session.Slice, 0, len(sgobs))
		for _, v := range sgobs {
			s := session.Session{}
			if err = gob.NewDecoder(bytes.NewBufferString(v)).Decode(&s); err != nil {
				return nil, governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
			}
			sarr = append(sarr, s)
		}
	} else {
		return nil, governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}
	sort.Sort(sort.Reverse(sarr))

	return &resUserGetSessions{
		Sessions: sarr,
	}, nil
}
