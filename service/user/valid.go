package user

import (
	"net/http"
	"net/mail"
	"regexp"

	"xorkevin.dev/governor"
	"xorkevin.dev/hunter2/h2otp"
)

//go:generate forge validation

const (
	lengthCapUserid       = 31
	lengthCapUsername     = 127
	lengthCapEmail        = 254
	lengthCapPassword     = 255
	lengthCapOTPCode      = 31
	lengthCapSessionID    = 31
	lengthCapRefreshToken = 255
	lengthCapName         = 127
	lengthCapToken        = 127
	amountCap             = 255
	lengthCapRole         = 127
	lengthCapApikeyid     = 31
	lengthCapScope        = 1024
)

var (
	userRegex = regexp.MustCompile(`^[a-z0-9_-]+$`)
	roleRegex = regexp.MustCompile(`^[a-z0-9._-]+$`)
)

func validoptUserid(userid string) error {
	if len(userid) > lengthCapUserid {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Userid must be shorter than 32 characters")
	}
	return nil
}

func validhasUserid(userid string) error {
	if len(userid) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Userid must be provided")
	}
	if err := validoptUserid(userid); err != nil {
		return err
	}
	return nil
}

func validhasUserids(userids []string) error {
	if len(userids) > amountCap {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Userids amount must be less than 256")
	}
	for _, i := range userids {
		if err := validhasUserid(i); err != nil {
			return err
		}
	}
	return nil
}

func validoptUsername(username string) error {
	if len(username) > lengthCapUsername {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Username must be shorter than 128 characters")
	}
	return nil
}

func validhasUsername(username string) error {
	if username == "" {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Username must be provided")
	}
	return validoptUsername(username)
}

func validUsername(username string) error {
	if err := validhasUsername(username); err != nil {
		return err
	}
	if len(username) < 3 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Username must be longer than 2 characters")
	}
	if !userRegex.MatchString(username) {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Username contains invalid characters")
	}
	return nil
}

func validEmail(email string) error {
	if len(email) > lengthCapEmail {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Email must be shorter than 255 characters")
	}
	a, err := mail.ParseAddress(email)
	if err != nil {
		return governor.ErrWithRes(err, http.StatusBadRequest, "", "Email is invalid")
	}
	if a.Address != email {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Email is invalid")
	}
	return nil
}

func validoptEmail(email string) error {
	if email == "" {
		return nil
	}
	return validEmail(email)
}

func validhasPassword(password string) error {
	if len(password) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Password must be provided")
	}
	if len(password) > lengthCapPassword {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Password entropy exceeds that of stored password hash")
	}
	return nil
}

func validPassword(password string) error {
	if err := validhasPassword(password); err != nil {
		return err
	}
	// min length checked by pass min size
	return nil
}

func validoptOTPCode(code string) error {
	if len(code) > lengthCapOTPCode {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Invalid otp code")
	}
	return nil
}

func validoptSessionID(sessionID string) error {
	if len(sessionID) > lengthCapSessionID {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Session id is too long")
	}
	return nil
}

func validhasSessionID(id string) error {
	if id == "" {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Session id must be provided")
	}
	if len(id) > lengthCapSessionID {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Session id is too long")
	}
	return nil
}

func validhasRefreshToken(token string) error {
	if len(token) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Refresh token must be provided")
	}
	if len(token) > lengthCapRefreshToken {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Refresh token is too long")
	}
	return nil
}

func validFirstName(firstname string) error {
	if len(firstname) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "First name must be provided")
	}
	if len(firstname) > lengthCapName {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "First name must be shorter than 128 characters")
	}
	return nil
}

func validLastName(lastname string) error {
	if len(lastname) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Last name must be provided")
	}
	if len(lastname) > lengthCapName {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Last name must be shorter than 128 characters")
	}
	return nil
}

func validhasToken(token string) error {
	if len(token) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Token must be provided")
	}
	if len(token) > lengthCapToken {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Token is too long")
	}
	return nil
}

func validAmount(amt int) error {
	if amt < 1 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Amount must be positive")
	}
	if amt > amountCap {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Amount must be less than 256")
	}
	return nil
}

func validOffset(offset int) error {
	if offset < 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Offset must not be negative")
	}
	return nil
}

func validhasRole(role string) error {
	if len(role) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Role is invalid")
	}
	if len(role) > lengthCapRole {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Role must be shorter than 128 characters")
	}
	return nil
}

func validRole(role string) error {
	if err := validhasRole(role); err != nil {
		return err
	}
	if !roleRegex.MatchString(role) {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Role contains invalid characters")
	}
	return nil
}

func validhasRolePrefix(prefix string) error {
	if len(prefix) > lengthCapRole {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Role prefix must be shorter than 128 characters")
	}
	return nil
}

func validRoles(roles []string) error {
	if len(roles) > amountCap {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Roles amount must be less than 256")
	}
	for _, i := range roles {
		if err := validRole(i); err != nil {
			return err
		}
	}
	return nil
}

func validScope(scopeString string) error {
	if len(scopeString) > lengthCapScope {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Scope string must be shorter than 4096 characters")
	}
	return nil
}

func validhasApikeyid(keyid string) error {
	if len(keyid) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Apikey id must be provided")
	}
	if len(keyid) > lengthCapApikeyid {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Apikey id must be shorter than 64 characters")
	}
	return nil
}

func validApikeyName(name string) error {
	if len(name) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Name must be provided")
	}
	if len(name) > lengthCapName {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Name must be shorter than 128 characters")
	}
	return nil
}

func validApikeyDesc(desc string) error {
	if len(desc) > lengthCapName {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Description must be shorter than 128 characters")
	}
	return nil
}

func validOTPAlg(alg string) error {
	_, ok := h2otp.DefaultHashes[alg]
	if !ok {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Invalid otp hash alg")
	}
	return nil
}

func validOTPDigits(digits int) error {
	switch digits {
	case 6, 8:
	default:
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Invalid otp digits")
	}
	return nil
}
