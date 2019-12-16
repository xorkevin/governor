package approvalmodel

import (
	"net/http"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/user/model"
)

//go:generate forge model -m Model -t userapprovals -p approval -o model_gen.go Model

type (
	Repo interface {
		New(m *usermodel.Model) *Model
		GetByID(userid string) (*Model, error)
		GetGroup(limit, offset int) ([]Model, error)
		Insert(m *Model) error
		Delete(m *Model) error
		Setup() error
	}

	repo struct {
		db db.Database
	}

	Model struct {
		Userid       string `model:"userid,VARCHAR(31) PRIMARY KEY" query:"userid,getoneeq,userid;deleq,userid"`
		Username     string `model:"username,VARCHAR(255) NOT NULL" query:"username"`
		AuthTags     string `model:"authtags,VARCHAR(4096) NOT NULL" query:"authtags"`
		PassHash     string `model:"pass_hash,VARCHAR(255) NOT NULL" query:"pass_hash"`
		Email        string `model:"email,VARCHAR(255) NOT NULL" query:"email"`
		FirstName    string `model:"first_name,VARCHAR(255) NOT NULL" query:"first_name"`
		LastName     string `model:"last_name,VARCHAR(255) NOT NULL" query:"last_name"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL" query:"creation_time,getgroup"`
	}
)

func New(database db.Database) Repo {
	return &repo{
		db: database,
	}
}

func (r *repo) New(m *usermodel.Model) *Model {
	return &Model{
		Userid:       m.Userid,
		Username:     m.Username,
		AuthTags:     m.AuthTags.Stringify(),
		PassHash:     m.PassHash,
		Email:        m.Email,
		FirstName:    m.FirstName,
		LastName:     m.LastName,
		CreationTime: m.CreationTime,
	}
}

func (r *repo) GetByID(userid string) (*Model, error) {
	m, code, err := approvalModelGetModelEqUserid(r.db.DB(), userid)
	if err != nil {
		if code == 2 {
			return nil, governor.NewError("No user found with that id", http.StatusNotFound, err)
		}
		return nil, governor.NewError("Failed to get user", http.StatusInternalServerError, err)
	}
	return m, nil
}

func (r *repo) GetGroup(limit, offset int) ([]Model, error) {
	m, err := approvalModelGetModelOrdCreationTime(r.db.DB(), true, limit, offset)
	if err != nil {
		return nil, governor.NewError("Failed to get user approvals", http.StatusInternalServerError, err)
	}
	return m, nil
}

func (r *repo) Insert(m *Model) error {
	if code, err := approvalModelInsert(r.db.DB(), m); err != nil {
		if code == 3 {
			return governor.NewError("User id must be unique", http.StatusBadRequest, err)
		}
		return governor.NewError("Failed to insert user", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *repo) Delete(m *Model) error {
	if err := approvalModelDelEqUserid(r.db.DB(), m.Userid); err != nil {
		return governor.NewError("Failed to delete user approval", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *repo) Setup() error {
	if err := approvalModelSetup(r.db.DB()); err != nil {
		return governor.NewError("Failed to setup user approval model", http.StatusInternalServerError, err)
	}
	return nil
}
