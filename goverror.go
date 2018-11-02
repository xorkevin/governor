package governor

import (
	"bytes"
	"github.com/labstack/echo"
	"net/http"
	"net/http/httputil"
)

const (
	moduleIDErr = "goverr"
)

type (
	// Error is an error container
	Error struct {
		origin   string
		source   []string
		message  string
		code     int
		status   int
		logLevel int
	}

	responseError struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	}
)

// NewError creates a new custom Error
func NewError(origin string, message string, code int, status int) *Error {
	return &Error{
		origin:   origin,
		source:   []string{origin},
		message:  message,
		code:     code,
		status:   status,
		logLevel: levelError,
	}
}

// NewErrorWarn creates a new custom Error
func NewErrorWarn(origin string, message string, code int, status int) *Error {
	return &Error{
		origin:   origin,
		source:   []string{origin},
		message:  message,
		code:     code,
		status:   status,
		logLevel: levelWarn,
	}
}

// NewErrorUser creates a new custom Error
func NewErrorUser(origin string, message string, code int, status int) *Error {
	return &Error{
		origin:   origin,
		source:   []string{origin},
		message:  message,
		code:     code,
		status:   status,
		logLevel: levelNoLog,
	}
}

func (e *Error) Error() string {
	return e.Message()
}

// Origin returns the origin of the error message
func (e *Error) Origin() string {
	return e.origin
}

// Source returns the source of the error message
func (e *Error) Source() string {
	k := bytes.NewBufferString(e.source[len(e.source)-1])
	for i := len(e.source) - 2; i > -1; i-- {
		k.WriteString("/")
		k.WriteString(e.source[i])
	}
	return k.String()
}

// Message returns the error message
func (e *Error) Message() string {
	return e.message
}

// Code returns the error code
func (e *Error) Code() int {
	return e.code
}

// Status returns the http status
func (e *Error) Status() int {
	return e.status
}

// Level returns the severity of the error
func (e *Error) Level() int {
	return e.logLevel
}

// IsErrorUser returns if the error is a user error
func (e *Error) IsErrorUser() bool {
	return e.logLevel == levelNoLog
}

// SetErrorUser makes the error a user error
func (e *Error) SetErrorUser() {
	e.logLevel = levelNoLog
}

// AddTrace adds the current caller to the call stack of the error
func (e *Error) AddTrace(module string) {
	e.source = append(e.source, module)
}

func errorHandler(i *echo.Echo, l Logger) echo.HTTPErrorHandler {
	return echo.HTTPErrorHandler(func(err error, c echo.Context) {
		origErr := err
		if err, ok := err.(*Error); ok {
			request := ""
			if r, reqerr := httputil.DumpRequest(c.Request(), true); reqerr == nil {
				request = bytes.NewBuffer(r).String()
			}
			switch err.Level() {
			case levelError:
				l.Error(err.Message(), err.Origin(), err.Message(), err.Code(), map[string]string{
					"source":   err.Source(),
					"endpoint": c.Path(),
					"request":  request,
				})
			case levelWarn:
				l.Warn(err.Message(), err.Origin(), err.Message(), err.Code(), map[string]string{
					"source":   err.Source(),
					"endpoint": c.Path(),
					"request":  request,
				})
			}
			if err := c.JSON(err.Status(), &responseError{
				Message: err.Message(),
				Code:    err.Code(),
			}); err != nil {
				gerr := NewError(moduleIDErr, err.Error(), 0, http.StatusInternalServerError)
				l.Warn(gerr.Message(), gerr.Origin(), "fail return error response", gerr.Code(), map[string]string{
					"source":   gerr.Source(),
					"endpoint": c.Path(),
					"request":  request,
				})
			}
		} else {
			i.DefaultHTTPErrorHandler(origErr, c)
		}
	})
}
