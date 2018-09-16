package user

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/role/model"
)

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
