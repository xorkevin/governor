package governor

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

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
func (e *ErrorRes) WriteError(b io.Writer) {
	io.WriteString(b, "(")
	io.WriteString(b, strconv.Itoa(e.Status))
	io.WriteString(b, ") ")
	io.WriteString(b, e.Message)
	if e.Code != "" {
		io.WriteString(b, " [")
		io.WriteString(b, e.Code)
		io.WriteString(b, "]")
	}
}

// Error implements error
func (e *ErrorRes) Error() string {
	b := strings.Builder{}
	e.WriteError(&b)
	return b.String()
}

type (
	// ErrorUnreachable is an error kind to mark unreachable code
	ErrorUnreachable struct{}
)

// Error implements error
func (e ErrorUnreachable) Error() string {
	return "Unreachable code. Invariant violated"
}

const (
	retryAfterHeader           = "Retry-After"
	retryAfterHeaderTimeFormat = "Mon, 02 Jan 2006 15:04:05 MST"
)

type (
	// ErrorTooManyRequests is an error kind to mark too many requests
	ErrorTooManyRequests struct {
		RetryAfter time.Time
	}
)

// Error implements error
func (e *ErrorTooManyRequests) Error() string {
	return fmt.Sprintf("Too many requests. Try again after %s.", e.RetryAfterTime())
}

// RetryAfterTime returns the earliest time at which the request may be retried
func (e *ErrorTooManyRequests) RetryAfterTime() string {
	return e.RetryAfter.Format(retryAfterHeaderTimeFormat)
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

// ErrWithUnreachable returns an error wrapped by an [*xorkevin.dev/kerrors.Error] with an [ErrorUnreachable] kind and message
func ErrWithUnreachable(err error, msg string) error {
	return kerrors.WithKind(err, ErrorUnreachable{}, msg)
}

// ErrWithTooManyRequests returns an error wrapped by [ErrWithRes] with an [ErrorTooManyRequests] kind and message
func ErrWithTooManyRequests(err error, t time.Time, code string, resmsg string) error {
	return ErrWithRes(kerrors.WithKind(err, &ErrorTooManyRequests{
		RetryAfter: t,
	}, "Too many requests"), http.StatusTooManyRequests, code, resmsg)
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

	var tmrErr *ErrorTooManyRequests
	if errors.As(err, &tmrErr) {
		c.SetHeader(retryAfterHeader, tmrErr.RetryAfterTime())
	}

	c.WriteJSON(rerr.Status, rerr)
}
