package governor

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"xorkevin.dev/kerrors"
)

// Sentinel errors
var (
	// ErrNoLog is an error kind to prevent logging
	ErrNoLog errNoLog
	// ErrUnreachable is an error kind to mark unreachable code
	ErrUnreachable errUnreachable
)

type (
	errNoLog struct{}
)

func (e errNoLog) Error() string {
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
	var b strings.Builder
	e.WriteError(&b)
	return b.String()
}

type (
	errUnreachable struct{}
)

func (e errUnreachable) Error() string {
	return "Unreachable code. Invariant violated"
}

const (
	retryAfterHeader = "Retry-After"
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
	return e.RetryAfter.UTC().Format(time.RFC1123)
}

// ErrWithNoLog returns an error wrapped by an [*xorkevin.dev/kerrors.Error] with an [ErrorNoLog] kind and message
func ErrWithNoLog(err error) error {
	return kerrors.New(
		kerrors.OptMsg("No log"),
		kerrors.OptKind(ErrNoLog),
		kerrors.OptInner(err),
		kerrors.OptSkip(1),
	)
}

// ErrWithRes returns an error wrapped by an [*xorkevin.dev/kerrors.Error] with an [ErrorRes] kind and message
func ErrWithRes(err error, status int, code string, resmsg string) error {
	return kerrors.New(
		kerrors.OptMsg("Error response"),
		kerrors.OptKind(&ErrorRes{
			Status:  status,
			Code:    code,
			Message: resmsg,
		}),
		kerrors.OptInner(err),
		kerrors.OptSkip(1),
	)
}

// ErrWithUnreachable returns an error wrapped by an [*xorkevin.dev/kerrors.Error] with an [ErrorUnreachable] kind and message
func ErrWithUnreachable(err error, msg string) error {
	return kerrors.New(
		kerrors.OptMsg(msg),
		kerrors.OptKind(ErrUnreachable),
		kerrors.OptInner(err),
		kerrors.OptSkip(1),
	)
}

// ErrWithTooManyRequests returns an error wrapped by [ErrWithRes] with an [ErrorTooManyRequests] kind and message
func ErrWithTooManyRequests(err error, t time.Time, code string, resmsg string) error {
	return ErrWithRes(kerrors.New(
		kerrors.OptMsg("Too many requests"),
		kerrors.OptKind(&ErrorTooManyRequests{
			RetryAfter: t,
		}),
		kerrors.OptInner(err),
		kerrors.OptSkip(1),
	), http.StatusTooManyRequests, code, resmsg)
}
