package governor

import (
	"errors"
	"fmt"
	"net/http"
)

type (
	goverror struct {
		message string
		status  int
		err     error
		noLog   bool
	}
)

func newGovError(message string, status int, err error) *goverror {
	if err == nil {
		return &goverror{
			message: message,
			status:  status,
			err:     nil,
		}
	}
	if message == "" || status == 0 {
		m := ""
		st := 0
		goverr := &goverror{}
		if errors.As(err, &goverr) {
			m = goverr.message
			st = goverr.status
		} else {
			m = err.Error()
		}
		if message == "" {
			message = m
		}
		if status == 0 {
			status = st
		}
	}
	return &goverror{
		message: message,
		status:  status,
		err:     err,
	}
}

// NewError creates a new governor Error
func NewError(message string, status int, err error) error {
	return newGovError(message, status, err)
}

// NewErrorUser creates a new governor User Error
func NewErrorUser(message string, status int, err error) error {
	gerr := newGovError(message, status, err)
	gerr.noLog = true
	return gerr
}

// Error formats the messages of all wrapped errors
func (e *goverror) Error() string {
	if e.err == nil {
		return e.message
	}
	return fmt.Sprintf("%s: %s", e.message, e.err.Error())
}

func (e *goverror) Unwrap() error {
	return e.err
}

func (e *goverror) Is(target error) bool {
	_, ok := target.(*goverror)
	return ok
}

func (e *goverror) As(target interface{}) bool {
	err, ok := target.(*goverror)
	if !ok {
		return false
	}
	err.message = e.message
	err.status = e.status
	err.err = e.err
	err.noLog = e.noLog
	return true
}

// ErrorStatus reports the error status of the top most governor error in the chain
func ErrorStatus(target error) int {
	if goverr := (&goverror{}); errors.As(target, &goverr) {
		return goverr.status
	}
	return 0
}

// ErrorMsg reports the error status of the top most governor error in the chain
func ErrorMsg(target error) string {
	if goverr := (&goverror{}); errors.As(target, &goverr) {
		return goverr.message
	}
	return ""
}

type (
	responseError struct {
		Message string `json:"message"`
	}
)

func (c *govcontext) WriteError(err error) {
	if gerr := (&goverror{}); errors.As(err, &gerr) {
		if c.l != nil && !gerr.noLog {
			c.l.Error(gerr.message, map[string]string{
				"endpoint": c.r.URL.EscapedPath(),
				"error":    gerr.Error(),
			})
		}
		status := http.StatusInternalServerError
		if s := gerr.status; s != 0 {
			status = s
		}
		c.WriteJSON(status, responseError{
			Message: gerr.message,
		})
		return
	}

	if c.l != nil {
		c.l.Error("non governor error", map[string]string{
			"endpoint": c.r.URL.EscapedPath(),
			"error":    err.Error(),
		})
	}
	c.WriteJSON(http.StatusInternalServerError, responseError{
		Message: "Internal Server Error",
	})
}
