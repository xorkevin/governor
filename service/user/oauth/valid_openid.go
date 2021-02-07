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
		return governor.NewCodeErrorUser(oidErrorInvalidScope, "Scope must be less than 256 characters", http.StatusBadRequest, nil)
	}
	for _, i := range strings.Fields(scope) {
		if i == oidScopeOpenid {
			return nil
		}
	}
	return governor.NewCodeErrorUser(oidErrorInvalidScope, "Invalid OpenID scope", http.StatusBadRequest, nil)
}

func validOidNonce(nonce string) error {
	if len(nonce) > lengthCapQuery {
		return governor.NewCodeErrorUser(oidErrorInvalidRequest, "Nonce must be less than 256 characters", http.StatusBadRequest, nil)
	}
	if !printableRegex.MatchString(nonce) {
		return governor.NewCodeErrorUser(oidErrorInvalidRequest, "Invalid nonce", http.StatusBadRequest, nil)
	}
	return nil
}

func validOidCodeChallenge(challenge string) error {
	if challenge == "" {
		return nil
	}
	if len(challenge) > lengthCapChallenge {
		return governor.NewCodeErrorUser(oidErrorInvalidRequest, "Code challenge must be less than 129 characters", http.StatusBadRequest, nil)
	}
	if len(challenge) < lengthFloorChallenge {
		return governor.NewCodeErrorUser(oidErrorInvalidRequest, "Code challenge must be greater than 42 characters", http.StatusBadRequest, nil)
	}
	if !codeChallengeRegex.MatchString(challenge) {
		return governor.NewCodeErrorUser(oidErrorInvalidRequest, "Invalid code challenge", http.StatusBadRequest, nil)
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
		return governor.NewCodeErrorUser(oidErrorInvalidRequest, "Invalid code challenge method", http.StatusBadRequest, nil)
	}
}

func validOidGrantType(grantType string) error {
	if grantType == "" {
		return governor.NewCodeErrorUser(oidErrorInvalidRequest, "Grant type must be provided", http.StatusBadRequest, nil)
	}
	switch grantType {
	case oidGrantTypeCode, oidGrantTypeRefresh:
		return nil
	default:
		return governor.NewCodeErrorUser(oidErrorUnsupportedGrant, "Invalid grant type", http.StatusBadRequest, nil)
	}
}

func validhasOidClientID(clientid string) error {
	if len(clientid) == 0 {
		return governor.NewCodeErrorUser(oidErrorInvalidRequest, "Client id must be provided", http.StatusBadRequest, nil)
	}
	if len(clientid) > lengthCapClientID {
		return governor.NewCodeErrorUser(oidErrorInvalidRequest, "Client id must be shorter than 32 characters", http.StatusBadRequest, nil)
	}
	return nil
}

func validhasOidClientSecret(secret string) error {
	if len(secret) == 0 {
		return governor.NewCodeErrorUser(oidErrorInvalidRequest, "Client secret must be provided", http.StatusBadRequest, nil)
	}
	if len(secret) > lengthCapQuery {
		return governor.NewCodeErrorUser(oidErrorInvalidRequest, "Client secret must be less than 256 characters", http.StatusBadRequest, nil)
	}
	return nil
}

func validhasOidUserid(userid string) error {
	if len(userid) == 0 {
		return governor.NewCodeErrorUser(oidErrorInvalidRequest, "Invalid authorization code", http.StatusBadRequest, nil)
	}
	if len(userid) > lengthCapUserid {
		return governor.NewCodeErrorUser(oidErrorInvalidRequest, "Invalid authorization code", http.StatusBadRequest, nil)
	}
	return nil
}

func validhasOidCode(code string) error {
	if len(code) == 0 {
		return governor.NewCodeErrorUser(oidErrorInvalidRequest, "Invalid authorization code", http.StatusBadRequest, nil)
	}
	if len(code) > lengthCapQuery {
		return governor.NewCodeErrorUser(oidErrorInvalidRequest, "Invalid authorization code", http.StatusBadRequest, nil)
	}
	return nil
}

func validhasOidRedirect(rawurl string) error {
	if len(rawurl) == 0 {
		return governor.NewCodeErrorUser(oidErrorInvalidRequest, "Redirect URI must be provided", http.StatusBadRequest, nil)
	}
	if len(rawurl) > lengthCapRedirect {
		return governor.NewCodeErrorUser(oidErrorInvalidRequest, "Redirect URI must be shorter than 513 characters", http.StatusBadRequest, nil)
	}
	return nil
}

func validoptOidCodeVerifier(code string) error {
	if len(code) > lengthCapQuery {
		return governor.NewCodeErrorUser(oidErrorInvalidRequest, "Code verifier must be less than 256 characters", http.StatusBadRequest, nil)
	}
	return nil
}
