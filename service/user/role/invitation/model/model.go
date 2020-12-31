package invitationmodel

import (
	"net/http"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/rank"
)

//go:generate forge model -m Model -t userroleinvitations -p inv -o model_gen.go Model

type (
	Repo interface {
		GetByID(userid, role string, after int64) (*Model, error)
		GetByUser(userid string, after int64, limit, offset int) ([]Model, error)
		GetByRole(role string, after int64, limit, offset int) ([]Model, error)
		GetByRolePrefix(prefix string, after int64, limit, offset int) ([]Model, error)
		Insert(userid string, roles rank.Rank, by string) error
		DeleteByID(userid, role string) error
		DeleteByRoles(userid string, roles rank.Rank) error
	}

	repo struct {
		db db.Database
	}

	// Model is the db role invitation model
	Model struct {
		Userid       string `model:"userid,VARCHAR(31);index" query:"userid"`
		Role         string `model:"role,VARCHAR(255), PRIMARY KEY (userid, role);index" query:"role,getoneeq,userid,role,creation_time|gt;deleq,userid,role;deleq,userid,role|arr"`
		InvitedBy    string `model:"invited_by,VARCHAR(31) NOT NULL" query:"invited_by"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL;index" query:"creation_time,getgroupeq,userid,creation_time|gt;getgroupeq,role,creation_time|gt;getgroupeq,role|like,creation_time|gt"`
	}
)

//func New(database db.Database) Repo {
//	return &repo{
//		db: database,
//	}
//}

func (r *repo) GetByID(userid, role string, after int64) (*Model, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, code, err := invModelGetModelEqUseridEqRoleGtCreationTime(db, userid, role, after)
	if err != nil {
		if code == 2 {
			return nil, governor.NewError("Invitation not found", http.StatusNotFound, err)
		}
		return nil, governor.NewError("Failed to get invitation", http.StatusInternalServerError, err)
	}
	return m, nil
}

// Setup creates a new role invitation table
func (r *repo) Setup() error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := invModelSetup(db); err != nil {
		return governor.NewError("Failed to setup role invitation model", http.StatusInternalServerError, err)
	}
	return nil
}
