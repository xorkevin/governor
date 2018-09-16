package user

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/model"
)

// GetUser gets and returns a user with the specified id
func (u *userService) GetUser(userid string) (*usermodel.Model, *governor.Error) {
	m, err := usermodel.GetByIDB64(u.db.DB(), userid)
	if err != nil {
		err.AddTrace(moduleID)
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

// GetByIdPublic gets and returns the public fields of the user
func (u *userService) GetByIdPublic(userid string) (*resUserGetPublic, *governor.Error) {
	m, err := usermodel.GetByIDB64(u.db.DB(), userid)
	if err != nil {
		err.AddTrace(moduleID)
		return nil, err
	}

	return &resUserGetPublic{
		Userid:       userid,
		Username:     m.Username,
		Tags:         m.Tags,
		FirstName:    m.FirstName,
		LastName:     m.LastName,
		CreationTime: m.CreationTime,
	}, nil
}

type (
	resUserGet struct {
		resUserGetPublic
		Email string `json:"email"`
	}
)

// GetById gets and returns all fields of the user
func (u *userService) GetById(userid string) (*resUserGet, *governor.Error) {
	m, err := usermodel.GetByIDB64(u.db.DB(), userid)
	if err != nil {
		err.AddTrace(moduleID)
		return nil, err
	}

	return &resUserGet{
		resUserGetPublic: resUserGetPublic{
			Userid:       userid,
			Username:     m.Username,
			Tags:         m.Tags,
			FirstName:    m.FirstName,
			LastName:     m.LastName,
			CreationTime: m.CreationTime,
		},
		Email: m.Email,
	}, nil
}
