package governor

import (
	"bytes"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
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

// AddTrace adds the current caller to the call stack of the error
func (e *Error) AddTrace(module string) {
	e.source = append(e.source, module)
}

func errorHandler(i *echo.Echo, l *logrus.Logger) echo.HTTPErrorHandler {
	return echo.HTTPErrorHandler(func(err error, c echo.Context) {
		origErr := err
		if err, ok := err.(*Error); ok {
			switch err.Level() {
			case levelError:
				l.WithFields(logrus.Fields{
					"origin": err.Origin(),
					"source": err.Source(),
					"code":   err.Code(),
				}).Error(err.Message())
			case levelWarn:
				l.WithFields(logrus.Fields{
					"origin": err.Origin(),
					"source": err.Source(),
					"code":   err.Code(),
				}).Warn(err.Message())
			}
			c.JSON(err.Status(), &responseError{
				Message: err.Message(),
				Code:    err.Code(),
			})
		} else {
			i.DefaultHTTPErrorHandler(origErr, c)
		}
	})
}
