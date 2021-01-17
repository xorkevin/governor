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
		return governor.NewErrorUser("Scope must be less than 256 characters", http.StatusBadRequest, nil)
	}
	for _, i := range strings.Fields(scope) {
		if i == oidScopeOpenid {
			return nil
		}
	}
	return governor.NewErrorUser("Invalid OpenID scope", http.StatusBadRequest, nil)
}

func validOidNonce(nonce string) error {
	if len(nonce) > lengthCapQuery {
		return governor.NewErrorUser("Nonce must be less than 256 characters", http.StatusBadRequest, nil)
	}
	if !printableRegex.MatchString(nonce) {
		return governor.NewErrorUser("Invalid nonce", http.StatusBadRequest, nil)
	}
	return nil
}

func validOidCodeChallenge(challenge string) error {
	if challenge == "" {
		return nil
	}
	if len(challenge) > lengthCapChallenge {
		return governor.NewErrorUser("Code challenge must be less than 129 characters", http.StatusBadRequest, nil)
	}
	if len(challenge) < lengthFloorChallenge {
		return governor.NewErrorUser("Code challenge must be greater than 42 characters", http.StatusBadRequest, nil)
	}
	if !codeChallengeRegex.MatchString(challenge) {
		return governor.NewErrorUser("Invalid code challenge", http.StatusBadRequest, nil)
	}
	return nil
}

func validOidCodeChallengeMethod(method string) error {
	switch method {
	case oidChallengePlain, oidChallengeS256:
		return nil
	default:
		return governor.NewErrorUser("Invalid code challenge method", http.StatusBadRequest, nil)
	}
}
