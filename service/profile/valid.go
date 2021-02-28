package profile

import (
	"net/http"
	"regexp"

	"xorkevin.dev/governor"
)

const (
	lengthCap      = 31
	lengthCapEmail = 255
	lengthCapLarge = 4095
)

var (
	emailRegex = regexp.MustCompile(`^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]+$`)
)

func validhasUserid(userid string) error {
	if len(userid) < 1 {
		return governor.NewErrorUser("Userid must be provided", http.StatusBadRequest, nil)
	}
	if len(userid) > lengthCap {
		return governor.NewErrorUser("Userid must be shorter than 32 characters", http.StatusBadRequest, nil)
	}
	return nil
}

func validEmail(email string) error {
	if len(email) == 0 {
		return nil
	}
	if !emailRegex.MatchString(email) {
		return governor.NewErrorUser("Email is invalid", http.StatusBadRequest, nil)
	}
	if len(email) > lengthCapEmail {
		return governor.NewErrorUser("Email must be shorter than 256 characters", http.StatusBadRequest, nil)
	}
	return nil
}

func validBio(bio string) error {
	if len(bio) > lengthCapLarge {
		return governor.NewErrorUser("Bio must be shorter than 4096 characters", http.StatusBadRequest, nil)
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
