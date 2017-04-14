package usermodel

type (
	// User is the db model
	User struct {
	}

	// ID holds all user identification information
	ID struct {
		UID      []byte `json:"uid"`
		Username string `json:"username"`
	}

	// Info holds user information
	Info struct {
	}

	// Props holds user facts
	Props struct {
	}
)
