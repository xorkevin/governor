package governor

import (
	"errors"
	"net/http"
	"strings"
)

type (
	// Error is a governor error
	Error struct {
		Message string
		Kind    error
		Inner   error
	}

	// ErrOpt is an error options function used by [NewError]
	ErrorOpt = func(e *Error)
)

// NewError creates a new governor [Error]
func NewError(opts ...ErrorOpt) error {
	e := &Error{}
	for _, i := range opts {
		i(e)
	}
	return e
}

// ErrOptMsg sets [Error.Message]
func ErrOptMsg(msg string) ErrorOpt {
	return func(e *Error) {
		e.Message = msg
	}
}

// ErrOptKind sets [Error.Kind]
func ErrOptKind(kind error) ErrorOpt {
	return func(e *Error) {
		e.Kind = kind
	}
}

// ErrOptInner sets [Error.Inner]
func ErrOptInner(inner error) ErrorOpt {
	return func(e *Error) {
		e.Inner = inner
	}
}

// Error implements error and recursively prints out wrapped errors
func (e *Error) Error() string {
	b := strings.Builder{}
	b.WriteString(e.Message)
	if e.Kind != nil {
		b.WriteString(" [")
		b.WriteString(e.Kind.Error())
		b.WriteString("]")
	}
	if e.Inner != nil {
		b.WriteString(": ")
		b.WriteString(e.Inner.Error())
	}
	return b.String()
}

// Unwrap returns the wrapped error
func (e *Error) Unwrap() error {
	return e.Inner
}

// Is recursively matches [Error.Kind]
func (e *Error) Is(target error) bool {
	if e.Kind == nil {
		return false
	}
	return errors.Is(e.Kind, target)
}

// As recursively matches [Error.Kind]
func (e *Error) As(target interface{}) bool {
	if e.Kind == nil {
		return false
	}
	return errors.As(e.Kind, target)
}

type (
	// ErrorStackTrace is a stack trace error kind
	ErrorStackTrace struct {
	}
)

func NewErrorStackTrace() *ErrorStackTrace {
	return &ErrorStackTrace{}
}

// Error implements error
func (e *ErrorStackTrace) Error() string {
	return "Error stack trace"
}

// ErrOptStackTrace sets the error kind to [ErrorStackTrace]
func ErrOptStackTrace(e *Error) {
	e.Kind = NewErrorStackTrace()
}

type (
	// ErrorNoLog is an error kind to prevent logging
	ErrorNoLog struct{}
)

// Error implements error
func (e ErrorNoLog) Error() string {
	return "No log"
}

// ErrOptNoLog sets the error kind to [ErrorNoLog]
func ErrOptNoLog(e *Error) {
	e.Kind = ErrorNoLog{}
}

type (
	// ErrorRes is an http error response kind
	ErrorRes struct {
		Status  int    `json:"-"`
		Code    string `json:"code,omitempty"`
		Message string `json:"message"`
	}
)

func (e *ErrorRes) Error() string {
	return e.Message
}

// ErrWithMsg returns a wrapped error with a message
func ErrWithMsg(err error, msg string) error {
	return NewError(ErrOptMsg(msg), ErrOptInner(err))
}

// ErrWithKind returns a wrapped error with a kind and message
func ErrWithKind(err error, kind error, msg string) error {
	return NewError(ErrOptMsg(msg), ErrOptKind(kind), ErrOptInner(err))
}

// ErrWithStackTrace returns a wrapped error with a [ErrorStackTrace] kind and message
func ErrWithStackTrace(err error, msg string) error {
	return NewError(ErrOptMsg(msg), ErrOptStackTrace, ErrOptInner(err))
}

// ErrWithNoLog returns a wrapped error with a [ErrorNoLog] kind and message
func ErrWithNoLog(err error, msg string) error {
	if msg == "" {
		msg = "No log"
	}
	return NewError(ErrOptMsg(msg), ErrOptNoLog, ErrOptInner(err))
}

// ErrWithRes returns a wrapped error with a [ErrorRes] kind and message
func ErrWithRes(err error, status int, code string, resmsg string, msg string) error {
	if msg == "" {
		msg = "Error response"
	}
	return NewError(ErrOptMsg(msg), ErrOptKind(&ErrorRes{
		Status:  status,
		Code:    code,
		Message: resmsg,
	}), ErrOptInner(err))
}

func (c *govcontext) WriteError(err error) {
	if c.l != nil && !errors.Is(err, ErrorNoLog{}) {
		msg := "non governor error"
		var gerr *Error
		if errors.As(err, &gerr) {
			msg = gerr.Message
		}
		c.l.Error(msg, map[string]string{
			"endpoint": c.r.URL.EscapedPath(),
			"error":    err.Error(),
		})
	}

	var rerr *ErrorRes
	if !errors.As(err, &rerr) {
		rerr = &ErrorRes{
			Status:  http.StatusInternalServerError,
			Message: "Internal Server Error",
		}
	}

	c.WriteJSON(rerr.Status, rerr)
}
