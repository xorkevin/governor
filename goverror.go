package governor

import (
	"bytes"
	"fmt"
	"github.com/labstack/echo"
	"golang.org/x/xerrors"
	"net/http"
	"net/http/httputil"
)

type (
	goverror struct {
		message string
		status  int
		err     error
	}
)

// NewError creates a new custom Error
func NewError(message string, status int, err error) error {
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
		goverruser := &goverrorUser{}
		if xerrors.As(err, &goverr) {
			m = goverr.message
			st = goverr.status
		} else if xerrors.As(err, &goverruser) {
			m = goverruser.message
			st = goverruser.status
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
	return true
}

type (
	goverrorUser struct {
		message string
		status  int
		err     error
	}
)

// NewErrorUser creates a new custom Error
func NewErrorUser(message string, status int, err error) error {
	if err == nil {
		return &goverrorUser{
			message: message,
			status:  status,
			err:     nil,
		}
	}
	if message == "" || status == 0 {
		m := ""
		st := 0
		goverruser := &goverrorUser{}
		goverr := &goverror{}
		if xerrors.As(err, &goverruser) {
			m = goverruser.message
			st = goverruser.status
		} else if xerrors.As(err, &goverr) {
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
	return &goverrorUser{
		message: message,
		status:  status,
		err:     err,
	}
}

func (e *goverrorUser) Error() string {
	if e.err == nil {
		return e.message
	}
	return fmt.Sprintf("%s: %s", e.message, e.err.Error())
}

func (e *goverrorUser) Unwrap() error {
	return e.err
}

func (e *goverrorUser) Is(target error) bool {
	_, ok := target.(*goverrorUser)
	return ok
}

func (e *goverrorUser) As(target interface{}) bool {
	err, ok := target.(*goverrorUser)
	if !ok {
		return false
	}
	err.message = e.message
	err.status = e.status
	err.err = e.err
	return true
}

type (
	responseError struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	}
)

func errorHandler(i *echo.Echo, l Logger) echo.HTTPErrorHandler {
	return echo.HTTPErrorHandler(func(err error, c echo.Context) {
		goverruser := &goverrorUser{}
		goverr := &goverror{}
		if xerrors.As(err, &goverruser) {
			status := http.StatusInternalServerError
			if goverruser.status != 0 {
				status = goverruser.status
			}
			if reserr := c.JSON(status, &responseError{
				Message: goverruser.message,
			}); reserr != nil {
				gerr := NewError("failed to send err message JSON", http.StatusInternalServerError, reserr)
				l.Warn(gerr.Error(), map[string]string{
					"endpoint": c.Path(),
				})
			}
		} else if xerrors.As(err, &goverr) {
			request := ""
			if r, reqerr := httputil.DumpRequest(c.Request(), true); reqerr == nil {
				request = bytes.NewBuffer(r).String()
			}
			l.Error(goverr.Error(), map[string]string{
				"endpoint": c.Path(),
				"request":  request,
			})
			status := http.StatusInternalServerError
			if goverr.status != 0 {
				status = goverr.status
			}
			if reserr := c.JSON(status, &responseError{
				Message: goverr.message,
			}); reserr != nil {
				gerr := NewError("failed to send err message JSON", http.StatusInternalServerError, reserr)
				l.Warn(gerr.Error(), map[string]string{
					"endpoint": c.Path(),
					"request":  request,
				})
			}
		} else {
			i.DefaultHTTPErrorHandler(err, c)
		}
	})
}
