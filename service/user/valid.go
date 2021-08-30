package user

import (
	"net/http"
	"regexp"
	"strings"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/hunter2"
)

const (
	lengthCapUserid   = 31
	lengthCapApikeyid = 63
	lengthCap         = 127
	lengthCapEmail    = 255
	lengthCapLarge    = 4095
	amountCap         = 255
	lengthCapOTPCode  = 31
)

var (
	userRegex  = regexp.MustCompile(`^[a-z][a-z0-9._-]+$`)
	emailRegex = regexp.MustCompile(`^[a-z0-9_-][a-z0-9_+-]*(\.[a-z0-9_+-]+)*@[a-z0-9]+(-+[a-z0-9]+)*(\.[a-z0-9]+(-+[a-z0-9]+)*)*$`)
)

func validhasUserid(userid string) error {
	if len(userid) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Userid must be provided",
		}))
	}
	if len(userid) > lengthCapUserid {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Userid must be shorter than 32 characters",
		}))
	}
	return nil
}

func validUsername(username string) error {
	if len(username) < 3 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Username must be longer than 2 characters",
		}))
	}
	if len(username) > lengthCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Username must be shorter than 128 characters",
		}))
	}
	if !userRegex.MatchString(username) {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Username contains invalid characters",
		}))
	}
	return nil
}

func validhasUsername(username string) error {
	if len(username) < 1 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Username must be provided",
		}))
	}
	if len(username) > lengthCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Username must be shorter than 128 characters",
		}))
	}
	return nil
}

func validoptUsername(username string) error {
	if len(username) > lengthCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Username must be shorter than 128 characters",
		}))
	}
	return nil
}

func validhasRole(role string) error {
	if len(role) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Role is invalid",
		}))
	}
	if len(role) > lengthCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Role must be shorter than 128 characters",
		}))
	}
	return nil
}

func validhasRolePrefix(prefix string) error {
	if len(prefix) > lengthCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Role prefix must be shorter than 128 characters",
		}))
	}
	return nil
}

func validAmount(amt int) error {
	if amt < 1 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Amount must be positive",
		}))
	}
	if amt > amountCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Amount must be less than 256",
		}))
	}
	return nil
}

func validOffset(offset int) error {
	if offset < 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Offset must not be negative",
		}))
	}
	return nil
}

func validhasUserids(userids []string) error {
	if len(userids) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "IDs must be provided",
		}))
	}
	if len(userids) > amountCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Request is too large",
		}))
	}
	for _, i := range userids {
		if err := validhasUserid(i); err != nil {
			return err
		}
	}
	return nil
}

func validPassword(password string) error {
	if len(password) < 10 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Password must be at least 10 chars",
		}))
	}
	if len(password) > lengthCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Password entropy exceeds that of stored password hash",
		}))
	}
	return nil
}

func validhasPassword(password string) error {
	if len(password) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Password must be provided",
		}))
	}
	if len(password) > lengthCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Password entropy exceeds that of stored password hash",
		}))
	}
	return nil
}

func validEmail(email string) error {
	if !emailRegex.MatchString(email) {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Email is invalid",
		}))
	}
	if len(email) > lengthCapEmail {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Email must be shorter than 256 characters",
		}))
	}
	return nil
}

func validFirstName(firstname string) error {
	if len(firstname) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "First name must be provided",
		}))
	}
	if len(firstname) > lengthCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "First name must be shorter than 128 characters",
		}))
	}
	return nil
}

func validLastName(lastname string) error {
	if len(lastname) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Last name must be provided",
		}))
	}
	if len(lastname) > lengthCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Last name must be shorter than 128 characters",
		}))
	}
	return nil
}

func validhasToken(token string) error {
	if len(token) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Token must be provided",
		}))
	}
	if len(token) > lengthCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Token is too long",
		}))
	}
	return nil
}

func validRank(rankSlice []string) error {
	if len(rankSlice) > lengthCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Rank update is too large",
		}))
	}
	return nil
}

func validRankStr(rankString string) error {
	if len(rankString) > lengthCapLarge {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Rank string is too large",
		}))
	}
	if _, err := rank.FromString(rankString); err != nil {
		return err
	}
	return nil
}

func validScope(scopeString string) error {
	if len(scopeString) > lengthCapLarge {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Scope must be shorter than 4096 characters",
		}))
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
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "SessionID must be provided",
		}))
	}
	if len(ids) > lengthCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Request is too large",
		}))
	}
	for _, i := range ids {
		if len(i) > lengthCap {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusBadRequest,
				Message: "SessionID is too large",
			}))
		}
	}
	return nil
}

func validhasSessionToken(token string) error {
	if len(token) > lengthCapLarge {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Token is too long",
		}))
	}
	return nil
}

func validhasRefreshToken(token string) error {
	if len(token) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Refresh token must be provided",
		}))
	}
	if len(token) > lengthCapLarge {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Token is too long",
		}))
	}
	return nil
}

func validhasApikeyid(keyid string) error {
	if len(keyid) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Apikey id must be provided",
		}))
	}
	if len(keyid) > lengthCapApikeyid {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Apikey id must be shorter than 64 characters",
		}))
	}
	return nil
}

func validApikeyName(name string) error {
	if len(name) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Name must be provided",
		}))
	}
	if len(name) > lengthCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Name must be shorter than 128 characters",
		}))
	}
	return nil
}

func validApikeyDesc(desc string) error {
	if len(desc) > lengthCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Description must be shorter than 128 characters",
		}))
	}
	return nil
}

func validOTPAlg(alg string) error {
	_, ok := hunter2.DefaultOTPHashes[alg]
	if !ok {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Invalid otp hash alg",
		}))
	}
	return nil
}

func validOTPDigits(digits int) error {
	switch digits {
	case 6, 8:
	default:
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Invalid otp digits",
		}))
	}
	return nil
}

func validOTPCode(code string) error {
	if len(code) > lengthCapOTPCode {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Invalid otp code",
		}))
	}
	return nil
}
