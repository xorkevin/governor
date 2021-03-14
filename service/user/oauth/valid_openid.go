package oauth

import (
	"net/http"
	"regexp"
	"strings"

	"xorkevin.dev/governor"
)

const (
	lengthCapQuery       = 255
	lengthCapChallenge   = 128
	lengthFloorChallenge = 43
)

var (
	printableRegex     = regexp.MustCompile(`^[[:print:]]*$`)
	codeChallengeRegex = regexp.MustCompile(`^[A-Za-z0-9._~-]*$`)
)

func validOidScope(scope string) error {
	if len(scope) > lengthCapQuery {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Code:    oidErrorInvalidScope,
			Message: "Scope must be less than 256 characters",
		}))
	}
	for _, i := range strings.Fields(scope) {
		if i == oidScopeOpenid {
			return nil
		}
	}
	return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
		Status:  http.StatusBadRequest,
		Code:    oidErrorInvalidScope,
		Message: "Invalid OpenID scope",
	}))
}

func validOidNonce(nonce string) error {
	if len(nonce) > lengthCapQuery {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Code:    oidErrorInvalidRequest,
			Message: "Nonce must be less than 256 characters",
		}))
	}
	if !printableRegex.MatchString(nonce) {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Code:    oidErrorInvalidRequest,
			Message: "Invalid nonce",
		}))
	}
	return nil
}

func validOidCodeChallenge(challenge string) error {
	if challenge == "" {
		return nil
	}
	if len(challenge) > lengthCapChallenge {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Code:    oidErrorInvalidRequest,
			Message: "Code challenge must be less than 129 characters",
		}))
	}
	if len(challenge) < lengthFloorChallenge {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Code:    oidErrorInvalidRequest,
			Message: "Code challenge must be greater than 42 characters",
		}))
	}
	if !codeChallengeRegex.MatchString(challenge) {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Code:    oidErrorInvalidRequest,
			Message: "Invalid code challenge",
		}))
	}
	return nil
}

func validOidCodeChallengeMethod(method string) error {
	if method == "" {
		return nil
	}
	switch method {
	case oidChallengePlain, oidChallengeS256:
		return nil
	default:
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Code:    oidErrorInvalidRequest,
			Message: "Invalid code challenge method",
		}))
	}
}

func validOidGrantType(grantType string) error {
	if grantType == "" {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Code:    oidErrorInvalidRequest,
			Message: "Grant type must be provided",
		}))
	}
	switch grantType {
	case oidGrantTypeCode, oidGrantTypeRefresh:
		return nil
	default:
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Code:    oidErrorUnsupportedGrant,
			Message: "Invalid grant type",
		}))
	}
}

func validhasOidClientID(clientid string) error {
	if len(clientid) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Code:    oidErrorInvalidRequest,
			Message: "Client id must be provided",
		}))
	}
	if len(clientid) > lengthCapClientID {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Code:    oidErrorInvalidRequest,
			Message: "Client id must be shorter than 32 characters",
		}))
	}
	return nil
}

func validhasOidClientSecret(secret string) error {
	if len(secret) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Code:    oidErrorInvalidRequest,
			Message: "Client secret must be provided",
		}))
	}
	if len(secret) > lengthCapQuery {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Code:    oidErrorInvalidRequest,
			Message: "Client secret must be less than 256 characters",
		}))
	}
	return nil
}

func validhasOidUserid(userid string) error {
	if len(userid) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Code:    oidErrorInvalidRequest,
			Message: "Invalid authorization code",
		}))
	}
	if len(userid) > lengthCapUserid {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Code:    oidErrorInvalidRequest,
			Message: "Invalid authorization code",
		}))
	}
	return nil
}

func validhasOidCode(code string) error {
	if len(code) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Code:    oidErrorInvalidRequest,
			Message: "Invalid authorization code",
		}))
	}
	if len(code) > lengthCapQuery {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Code:    oidErrorInvalidRequest,
			Message: "Invalid authorization code",
		}))
	}
	return nil
}

func validhasOidRedirect(rawurl string) error {
	if len(rawurl) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Code:    oidErrorInvalidRequest,
			Message: "Redirect URI must be provided",
		}))
	}
	if len(rawurl) > lengthCapRedirect {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Code:    oidErrorInvalidRequest,
			Message: "Redirect URI must be shorter than 513 characters",
		}))
	}
	return nil
}

func validoptOidCodeVerifier(code string) error {
	if len(code) > lengthCapQuery {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Code:    oidErrorInvalidRequest,
			Message: "Code verifier must be less than 256 characters",
		}))
	}
	return nil
}
