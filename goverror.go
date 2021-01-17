package governor

import (
	"errors"
	"fmt"
	"net/http"
)

type (
	goverror struct {
		code    string
		message string
		status  int
		err     error
		noLog   bool
	}
)

func newGovError(code, message string, status int, err error) *goverror {
	if err == nil {
		return &goverror{
			code:    code,
			message: message,
			status:  status,
			err:     nil,
		}
	}
	if code == "" || message == "" || status == 0 {
		c := ""
		m := ""
		st := 0
		goverr := &goverror{}
		if errors.As(err, &goverr) {
			c = goverr.code
			m = goverr.message
			st = goverr.status
		} else {
			m = err.Error()
		}
		if code == "" {
			code = c
		}
		if message == "" {
			message = m
		}
		if status == 0 {
			status = st
		}
	}
	return &goverror{
		code:    code,
		message: message,
		status:  status,
		err:     err,
	}
}

// NewError creates a new governor Error
func NewError(message string, status int, err error) error {
	return newGovError("", message, status, err)
}

// NewCodeError creates a new governor Error with an error code
func NewCodeError(code, message string, status int, err error) error {
	return newGovError(code, message, status, err)
}

// NewErrorUser creates a new governor User Error
func NewErrorUser(message string, status int, err error) error {
	gerr := newGovError("", message, status, err)
	gerr.noLog = true
	return gerr
}

// NewCodeErrorUser creates a new governor User Error with an error code
func NewCodeErrorUser(code, message string, status int, err error) error {
	gerr := newGovError(code, message, status, err)
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
	t, ok := target.(*goverror)
	if !ok {
		return false
	}
	return t.code == e.code
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

// ErrorCode reports the error code of the top most governor error in the chain
func ErrorCode(target error) string {
	if goverr := (&goverror{}); errors.As(target, &goverr) {
		return goverr.code
	}
	return ""
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
		Code    string `json:"code,omitempty"`
		Message string `json:"message"`
	}
)

func (c *govcontext) WriteError(err error) {
	if gerr := (&goverror{}); errors.As(err, &gerr) {
		if c.l != nil && !gerr.noLog {
			c.l.Error(gerr.message, map[string]string{
				"endpoint": c.r.URL.EscapedPath(),
				"error":    gerr.Error(),
				"code":     gerr.code,
			})
		}
		status := http.StatusInternalServerError
		if s := gerr.status; s != 0 {
			status = s
		}
		c.WriteJSON(status, responseError{
			Code:    gerr.code,
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
