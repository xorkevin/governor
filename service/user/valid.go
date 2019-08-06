package user

import (
	"net/http"
	"regexp"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/rank"
)

const (
	lengthCap      = 127
	lengthCapEmail = 255
	lengthCapLarge = 4095
	amountCap      = 1024
)

var (
	userRegex  = regexp.MustCompile(`^[a-z][a-z0-9._-]+$`)
	emailRegex = regexp.MustCompile(`^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]+$`)
)

func validhasUserid(userid string) error {
	if len(userid) == 0 {
		return governor.NewErrorUser("Userid must be provided", http.StatusBadRequest, nil)
	}
	if len(userid) > lengthCap {
		return governor.NewErrorUser("Userid is too long", http.StatusBadRequest, nil)
	}
	return nil
}

func validUsername(username string) error {
	if len(username) < 3 {
		return governor.NewErrorUser("Username must be longer than 2 chars", http.StatusBadRequest, nil)
	}
	if len(username) > lengthCap {
		return governor.NewErrorUser("Username is too long", http.StatusBadRequest, nil)
	}
	if !userRegex.MatchString(username) {
		return governor.NewErrorUser("Username contains invalid characters", http.StatusBadRequest, nil)
	}
	return nil
}

func validhasUsername(username string) error {
	if len(username) < 1 {
		return governor.NewErrorUser("Username must be provided", http.StatusBadRequest, nil)
	}
	if len(username) > lengthCap {
		return governor.NewErrorUser("Username is too long", http.StatusBadRequest, nil)
	}
	return nil
}

func validhasRole(role string) error {
	if len(role) == 0 {
		return governor.NewErrorUser("Role is invalid", http.StatusBadRequest, nil)
	}
	if len(role) > lengthCap {
		return governor.NewErrorUser("Role is too long", http.StatusBadRequest, nil)
	}
	return nil
}

func validAmount(amt int) error {
	if amt == 0 || amt > amountCap {
		return governor.NewErrorUser("Amount is invalid", http.StatusBadRequest, nil)
	}
	return nil
}

func validOffset(offset int) error {
	if offset < 0 {
		return governor.NewErrorUser("Offset is invalid", http.StatusBadRequest, nil)
	}
	return nil
}

func validhasUserids(userids string) error {
	if len(userids) == 0 {
		return governor.NewErrorUser("IDs must be provided", http.StatusBadRequest, nil)
	}
	if len(userids) > lengthCapLarge {
		return governor.NewErrorUser("Request is too large", http.StatusBadRequest, nil)
	}
	return nil
}

func validPassword(password string) error {
	if len(password) < 10 {
		return governor.NewErrorUser("Password must be at least 10 chars", http.StatusBadRequest, nil)
	}
	if len(password) > lengthCap {
		return governor.NewErrorUser("Password entropy exceeds that of stored password hash", http.StatusBadRequest, nil)
	}
	return nil
}

func validhasPassword(password string) error {
	if len(password) == 0 {
		return governor.NewErrorUser("Password must be provided", http.StatusBadRequest, nil)
	}
	if len(password) > lengthCap {
		return governor.NewErrorUser("Password entropy exceeds that of stored password hash", http.StatusBadRequest, nil)
	}
	return nil
}

func validEmail(email string) error {
	if !emailRegex.MatchString(email) || len(email) > lengthCapEmail {
		return governor.NewErrorUser("Email is invalid", http.StatusBadRequest, nil)
	}
	return nil
}

func validFirstName(firstname string) error {
	if len(firstname) == 0 {
		return governor.NewErrorUser("First name must be provided", http.StatusBadRequest, nil)
	}
	if len(firstname) > lengthCap {
		return governor.NewErrorUser("First name is too long", http.StatusBadRequest, nil)
	}
	return nil
}

func validLastName(lastname string) error {
	if len(lastname) == 0 {
		return governor.NewErrorUser("Last name must be provided", http.StatusBadRequest, nil)
	}
	if len(lastname) > lengthCap {
		return governor.NewErrorUser("Last name is too long", http.StatusBadRequest, nil)
	}
	return nil
}

func validhasToken(token string) error {
	if len(token) == 0 {
		return governor.NewErrorUser("Token must be provided", http.StatusBadRequest, nil)
	}
	if len(token) > lengthCap {
		return governor.NewErrorUser("Token is too long", http.StatusBadRequest, nil)
	}
	return nil
}

func validRank(rankString string) error {
	if len(rankString) > lengthCapLarge {
		return governor.NewErrorUser("Rank update is too large", http.StatusBadRequest, nil)
	}
	if _, err := rank.FromStringUser(rankString); err != nil {
		return err
	}
	return nil
}

func validhasUsernameOrEmail(useroremail string) (bool, error) {
	if err := validEmail(useroremail); err == nil {
		return true, nil
	}
	if err := validhasUsername(useroremail); err == nil {
		return false, nil
	}
	return false, governor.NewErrorUser("Invalid username or email", http.StatusBadRequest, nil)
}

func validhasSessionIDs(ids []string) error {
	if len(ids) == 0 {
		return governor.NewErrorUser("SessionID must be provided", http.StatusBadRequest, nil)
	}
	if len(ids) > lengthCapLarge {
		return governor.NewErrorUser("Request is too large", http.StatusBadRequest, nil)
	}
	return nil
}

func validhasSessionToken(token string) error {
	if len(token) > lengthCapLarge {
		return governor.NewErrorUser("Token is too long", http.StatusBadRequest, nil)
	}
	return nil
}

func validhasRefreshToken(token string) error {
	if len(token) == 0 {
		return governor.NewErrorUser("Refresh token must be provided", http.StatusBadRequest, nil)
	}
	if len(token) > lengthCapLarge {
		return governor.NewErrorUser("Token is too long", http.StatusBadRequest, nil)
	}
	return nil
}
