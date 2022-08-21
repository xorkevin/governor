package governor

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"xorkevin.dev/kerrors"
)

type (
	// ErrorNoLog is an error kind to prevent logging
	ErrorNoLog struct{}
)

// Error implements error
func (e ErrorNoLog) Error() string {
	return "No log"
}

type (
	// ErrorRes is an http error response kind
	ErrorRes struct {
		Status  int    `json:"-"`
		Code    string `json:"code,omitempty"`
		Message string `json:"message"`
	}
)

// WriteError implements [xorkevin.dev/kerrors.ErrorWriter]
func (e *ErrorRes) WriteError(b io.StringWriter) {
	b.WriteString("(")
	b.WriteString(strconv.Itoa(e.Status))
	b.WriteString(") ")
	b.WriteString(e.Message)
	if e.Code != "" {
		b.WriteString(" [")
		b.WriteString(e.Code)
		b.WriteString("]")
	}
}

// Error implements error
func (e *ErrorRes) Error() string {
	b := strings.Builder{}
	e.WriteError(&b)
	return b.String()
}

// ErrWithNoLog returns an error wrapped by an [*xorkevin.dev/kerrors.Error] with an [ErrorNoLog] kind and message
func ErrWithNoLog(err error) error {
	return kerrors.WithKind(err, ErrorNoLog{}, "No log")
}

// ErrWithRes returns an error wrapped by an [*xorkevin.dev/kerrors.Error] with an [ErrorRes] kind and message
func ErrWithRes(err error, status int, code string, resmsg string) error {
	return kerrors.WithKind(err, &ErrorRes{
		Status:  status,
		Code:    code,
		Message: resmsg,
	}, "Error response")
}

func (c *govcontext) WriteError(err error) {
	var rerr *ErrorRes
	if !errors.As(err, &rerr) {
		rerr = &ErrorRes{
			Status:  http.StatusInternalServerError,
			Message: "Internal Server Error",
		}
	}

	if c.l != nil && !errors.Is(err, ErrorNoLog{}) {
		msg := "non-kerror"
		var kerr *kerrors.Error
		if errors.As(err, &kerr) {
			msg = kerr.Message
		}
		stacktrace := "NONE"
		var serr *kerrors.StackTrace
		if errors.As(err, &serr) {
			stacktrace = serr.StackString()
		}
		if rerr.Status >= http.StatusBadRequest && rerr.Status < http.StatusInternalServerError {
			c.l.Warn(msg, map[string]string{
				"endpoint":   c.r.URL.EscapedPath(),
				"error":      err.Error(),
				"stacktrace": stacktrace,
			})
		} else {
			c.l.Error(msg, map[string]string{
				"endpoint":   c.r.URL.EscapedPath(),
				"error":      err.Error(),
				"stacktrace": stacktrace,
			})
		}
	}

	c.WriteJSON(rerr.Status, rerr)
}
