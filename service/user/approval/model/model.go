package approvalmodel

import ()

//go:generate forge model -m Model -t userapprovals -p approval -o model_gen.go Model

type (
	Repo interface {
		GetByID(userid string) (*Model, error)
		GetGroup(limit, offset int) ([]Model, error)
		Insert(m *Model) error
		Delete(m *Model) error
	}

	Model struct {
		Userid       string `model:"userid,VARCHAR(31) PRIMARY KEY" query:"userid,getoneeq,userid;deleq,userid"`
		Username     string `model:"username,VARCHAR(255) NOT NULL UNIQUE" query:"username"`
		AuthTags     string `model:"authtags,VARCHAR(4096) NOT NULL" query:"authtags"`
		PassHash     string `model:"pass_hash,VARCHAR(255) NOT NULL" query:"pass_hash"`
		Email        string `model:"email,VARCHAR(255) NOT NULL UNIQUE" query:"email"`
		FirstName    string `model:"first_name,VARCHAR(255) NOT NULL" query:"first_name"`
		LastName     string `model:"last_name,VARCHAR(255) NOT NULL" query:"last_name"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL" query:"creation_time,getgroup"`
	}
)
