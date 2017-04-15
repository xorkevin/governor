package usermodel

type (
	// Model is the db User model
	Model struct {
		id
		auth
		passhash
		props
	}

	id struct {
		uid
		username
	}

	uid struct {
		UID []byte `json:"uid"`
	}

	username struct {
		Username string `json:"username"`
	}

	auth struct {
		Level uint64   `json:"auth_level"`
		Tags  []string `json:"auth_tags"`
	}

	passhash struct {
		Hash    []byte `json:"pass_hash"`
		Salt    []byte `json:"pass_salt"`
		Version int    `json:"pass_version"`
	}

	props struct {
		name
		email
	}

	name struct {
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
	}

	email struct {
		Email string `json:"email"`
	}
)
