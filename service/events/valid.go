package events

import (
	"net/http"

	"xorkevin.dev/governor"
)

const (
	lengthCapSubject = 255
)

func validSubject(subject string) error {
	if len(subject) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Subject must be provided",
		}))
	}
	if len(subject) > lengthCapSubject {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Subject must be shorter than 256 characters",
		}))
	}
	return nil
}
