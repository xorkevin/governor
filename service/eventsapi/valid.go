package eventsapi

import (
	"net/http"

	"xorkevin.dev/governor"
)

//go:generate forge validation

const (
	lengthCapSubject = 255
)

func validSubject(subject string) error {
	if len(subject) == 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Subject must be provided")
	}
	if len(subject) > lengthCapSubject {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Subject must be shorter than 256 characters")
	}
	return nil
}
