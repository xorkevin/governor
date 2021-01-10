package oauth

import (
	"net/http"
	"regexp"
	"strings"
	"xorkevin.dev/governor"
)

const (
	lengthCapNonce = 2047
)

var (
	printableRegex = regexp.MustCompile(`^[[:print:]]*$`)
)

func validOidResponseType(responseType string) error {
	if responseType != oidResponseTypeCode {
		return governor.NewErrorUser("Invalid response type", http.StatusBadRequest, nil)
	}
	return nil
}

func validOidResponseMode(responseMode string) error {
	switch responseMode {
	case oidResponseModeQuery, oidResponseModeFragment:
		return nil
	default:
		return governor.NewErrorUser("Invalid response mode", http.StatusBadRequest, nil)
	}
}

func validOidScope(scope string) error {
	for _, i := range strings.Fields(scope) {
		if i == oidScopeOpenid {
			return nil
		}
	}
	return governor.NewErrorUser("Invalid OpenID scope", http.StatusBadRequest, nil)
}

func validOidState(state string) error {
	if len(state) > lengthCapNonce {
		return governor.NewErrorUser("State must be less than 2048 characters", http.StatusBadRequest, nil)
	}
	if !printableRegex.MatchString(state) {
		return governor.NewErrorUser("Invalid state", http.StatusBadRequest, nil)
	}
	return nil
}

func validOidNonce(nonce string) error {
	if len(nonce) > lengthCapNonce {
		return governor.NewErrorUser("Nonce must be less than 2048 characters", http.StatusBadRequest, nil)
	}
	if !printableRegex.MatchString(nonce) {
		return governor.NewErrorUser("Invalid nonce", http.StatusBadRequest, nil)
	}
	return nil
}

func validOidCodeChallenge(challenge string) error {
	if len(challenge) > lengthCapNonce {
		return governor.NewErrorUser("Code challenge must be less than 2048 characters", http.StatusBadRequest, nil)
	}
	if !printableRegex.MatchString(challenge) {
		return governor.NewErrorUser("Invalid nonce", http.StatusBadRequest, nil)
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

func validOidDisplay(display string) error {
	switch display {
	case oidDisplayPage,
		oidDisplayPopup,
		oidDisplayTouch,
		oidDisplayWap:
		return nil
	default:
		return governor.NewErrorUser("Invalid display", http.StatusBadRequest, nil)
	}
}

func validOidPrompt(prompt string) error {
	if prompt == "" {
		return nil
	}
	for _, i := range strings.Fields(prompt) {
		switch i {
		case oidPromptNone,
			oidPromptLogin,
			oidPromptConsent,
			oidPromptSelectAcct:
		default:
			return governor.NewErrorUser("Invalid prompt", http.StatusBadRequest, nil)
		}
	}
	return nil
}

func validOidMaxAge(age int) error {
	if age < -1 {
		return governor.NewErrorUser("Invalid max age", http.StatusBadRequest, nil)
	}
	return nil
}

func validOidLoginHint(hint string) error {
	if len(hint) > lengthCapNonce {
		return governor.NewErrorUser("Login hint must be less than 2048 characters", http.StatusBadRequest, nil)
	}
	return nil
}
