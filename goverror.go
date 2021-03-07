package governor

import (
	"errors"
	"net/http"
	"strings"
)

type (
	// Error is a governor error
	Error struct {
		Kind    error
		Status  int
		Code    string
		Message string
		Inner   error
	}

	// ErrOpt is a error options function
	ErrorOpt = func(e *Error)
)

// NewError creates a new governor error
func NewError(opts ...ErrorOpt) error {
	e := &Error{}
	for _, i := range opts {
		i(e)
	}
	return e
}

// ErrOptKind sets the error kind
func ErrOptKind(kind error) ErrorOpt {
	return func(e *Error) {
		e.Kind = kind
	}
}

// ErrOptInner sets the wrapped error
func ErrOptInner(inner error) ErrorOpt {
	return func(e *Error) {
		e.Inner = inner
	}
}

// ErrOptMsg sets the error message
func ErrOptMsg(msg string) ErrorOpt {
	return func(e *Error) {
		e.Message = msg
	}
}

// ErrOptRes sets the error response
func ErrOptRes(res ErrorRes) ErrorOpt {
	return func(e *Error) {
		e.Status = res.Status
		e.Code = res.Code
		e.Message = res.Message
	}
}

// Error formats the messages of all wrapped errors
func (e Error) Error() string {
	b := strings.Builder{}
	if e.Kind != nil {
		b.WriteString("[")
		b.WriteString(e.Kind.Error())
		b.WriteString("] ")
	}
	if e.Code != "" {
		b.WriteString(e.Code)
		b.WriteString(" ")
	}
	b.WriteString(e.Message)
	if e.Inner == nil {
		return b.String()
	}
	b.WriteString(": ")
	b.WriteString(e.Inner.Error())
	return b.String()
}

// Unwrap returns the wrapped error
func (e *Error) Unwrap() error {
	return e.Inner
}

// Is returns if the error kind is equal
func (e *Error) Is(target error) bool {
	if e.Kind == nil {
		return false
	}
	return errors.Is(e.Kind, target)
}

// As sets the top *Error or *ErrorRes
func (e *Error) As(target interface{}) bool {
	if err, ok := target.(*Error); ok {
		*err = *e
		return true
	}
	if e.Status == 0 {
		return false
	}
	if err, ok := target.(*ErrorRes); ok {
		err.Status = e.Status
		err.Code = e.Code
		err.Message = e.Message
		return true
	}
	return false
}

type (
	// ErrorUser is a user error
	ErrorUser struct{}
)

// Error implements error
func (e ErrorUser) Error() string {
	return "User error"
}

// ErrOptUser sets the error kind to a user error
func ErrOptUser(e *Error) {
	e.Kind = ErrorUser{}
}

type (
	// ErrorRes is an http error response
	ErrorRes struct {
		Status  int    `json:"-"`
		Code    string `json:"code,omitempty"`
		Message string `json:"message"`
	}
)

func (e ErrorRes) Error() string {
	return e.Message
}

func (c *govcontext) WriteError(err error) {
	gerr := &Error{}
	isError := errors.As(err, gerr)

	if c.l != nil && !errors.Is(err, ErrorUser{}) {
		msg := "non governor error"
		if isError {
			msg = gerr.Message
		}
		c.l.Error(msg, map[string]string{
			"endpoint": c.r.URL.EscapedPath(),
			"error":    err.Error(),
		})
	}

	rerr := &ErrorRes{}
	if !errors.As(err, rerr) {
		rerr.Status = http.StatusInternalServerError
		if isError {
			rerr.Code = gerr.Code
			rerr.Message = gerr.Message
		} else {
			rerr.Message = "Internal Server Error"
		}
	}

	c.WriteJSON(rerr.Status, rerr)
}
