package user

import (
	"net/http"
	"regexp"
	"strings"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/rank"
)

const (
	lengthCapUserid   = 31
	lengthCapApikeyid = 63
	lengthCap         = 127
	lengthCapEmail    = 255
	lengthCapLarge    = 4095
	amountCap         = 1024
)

var (
	userRegex  = regexp.MustCompile(`^[a-z][a-z0-9._-]+$`)
	emailRegex = regexp.MustCompile(`^[a-z0-9_-][a-z0-9_+-]*(\.[a-z0-9_+-]+)*@[a-z0-9]+(-+[a-z0-9]+)*(\.[a-z0-9]+(-+[a-z0-9]+)*)*$`)
)

func validhasUserid(userid string) error {
	if len(userid) == 0 {
		return governor.NewErrorUser("Userid must be provided", http.StatusBadRequest, nil)
	}
	if len(userid) > lengthCapUserid {
		return governor.NewErrorUser("Userid must be shorter than 32 characters", http.StatusBadRequest, nil)
	}
	return nil
}

func validUsername(username string) error {
	if len(username) < 3 {
		return governor.NewErrorUser("Username must be longer than 2 characters", http.StatusBadRequest, nil)
	}
	if len(username) > lengthCap {
		return governor.NewErrorUser("Username must be shorter than 128 characters", http.StatusBadRequest, nil)
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
		return governor.NewErrorUser("Username must be shorter than 128 characters", http.StatusBadRequest, nil)
	}
	return nil
}

func validhasRole(role string) error {
	if len(role) == 0 {
		return governor.NewErrorUser("Role is invalid", http.StatusBadRequest, nil)
	}
	if len(role) > lengthCap {
		return governor.NewErrorUser("Role must be shorter than 128 characters", http.StatusBadRequest, nil)
	}
	return nil
}

func validAmount(amt int) error {
	if amt == 0 {
		return governor.NewErrorUser("Amount must be positive", http.StatusBadRequest, nil)
	}
	if amt > amountCap {
		return governor.NewErrorUser("Amount must be less than 1024", http.StatusBadRequest, nil)
	}
	return nil
}

func validOffset(offset int) error {
	if offset < 0 {
		return governor.NewErrorUser("Offset must not be negative", http.StatusBadRequest, nil)
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
	if !emailRegex.MatchString(email) {
		return governor.NewErrorUser("Email is invalid", http.StatusBadRequest, nil)
	}
	if len(email) > lengthCapEmail {
		return governor.NewErrorUser("Email must be shorter than 256 characters", http.StatusBadRequest, nil)
	}
	return nil
}

func validFirstName(firstname string) error {
	if len(firstname) == 0 {
		return governor.NewErrorUser("First name must be provided", http.StatusBadRequest, nil)
	}
	if len(firstname) > lengthCap {
		return governor.NewErrorUser("First name must be shorter than 128 characters", http.StatusBadRequest, nil)
	}
	return nil
}

func validLastName(lastname string) error {
	if len(lastname) == 0 {
		return governor.NewErrorUser("Last name must be provided", http.StatusBadRequest, nil)
	}
	if len(lastname) > lengthCap {
		return governor.NewErrorUser("Last name must be shorter than 128 characters", http.StatusBadRequest, nil)
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

func isEmail(useroremail string) bool {
	return strings.ContainsRune(useroremail, '@')
}

func validhasUsernameOrEmail(useroremail string) error {
	if isEmail(useroremail) {
		return validEmail(useroremail)
	}
	return validhasUsername(useroremail)
}

func validSessionIDs(ids []string) error {
	if len(ids) == 0 {
		return governor.NewErrorUser("SessionID must be provided", http.StatusBadRequest, nil)
	}
	if len(ids) > lengthCap {
		return governor.NewErrorUser("Request is too large", http.StatusBadRequest, nil)
	}
	for _, i := range ids {
		if len(i) > lengthCap {
			return governor.NewErrorUser("SessionID is too large", http.StatusBadRequest, nil)
		}
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

func validhasApikeyid(keyid string) error {
	if len(keyid) == 0 {
		return governor.NewErrorUser("Apikey id must be provided", http.StatusBadRequest, nil)
	}
	if len(keyid) > lengthCapApikeyid {
		return governor.NewErrorUser("Apikey id must be shorter than 64 characters", http.StatusBadRequest, nil)
	}
	return nil
}

func validApikeyName(name string) error {
	if len(name) == 0 {
		return governor.NewErrorUser("Name must be provided", http.StatusBadRequest, nil)
	}
	if len(name) > lengthCap {
		return governor.NewErrorUser("Name must be shorter than 128 characters", http.StatusBadRequest, nil)
	}
	return nil
}

func validApikeyDesc(desc string) error {
	if len(desc) > lengthCap {
		return governor.NewErrorUser("Description must be shorter than 128 characters", http.StatusBadRequest, nil)
	}
	return nil
}
