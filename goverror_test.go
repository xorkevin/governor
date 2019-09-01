package governor

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/labstack/echo"
	"github.com/stretchr/testify/assert"
	"golang.org/x/xerrors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewError(t *testing.T) {
	assert := assert.New(t)

	rootErr := errors.New("test root err")

	{
		err := NewError("test message", 123, rootErr)
		assert.Error(err, "should not return an empty error")
		k := err.(*goverror)
		assert.Equal("test message", k.message, "error should have the message that was passed in")
		assert.Equal(123, k.status, "error should have the status that was passed in")
		assert.Equal(rootErr, k.err, "error should have the err that was passed in")

		err2 := NewError("", 0, err)
		assert.Error(err2, "should not return an empty error")
		k2 := err2.(*goverror)
		assert.Equal("test message", k2.message, "error should have its nearest goverror causer message by default")
		assert.Equal(123, k2.status, "error should have its nearest goverror causer status by default")
		assert.Equal(err, k2.err, "error should have the err that was passed in")
	}

	{
		err := NewError("", 0, rootErr)
		assert.Error(err, "should not return an empty error")
		k := err.(*goverror)
		assert.Equal("test root err", k.message, "error should have its nearest causer message if no goverror by default")
		assert.Equal(0, k.status, "error should have its nearest goverror causer status if no goverror by default")
		assert.Equal(rootErr, k.err, "error should have the err that was passed in")
	}

	{
		err := NewError("test message", 123, nil)
		assert.Error(err, "should not return an empty error")
		k := err.(*goverror)
		assert.Equal("test message", k.message, "error should have the message that was passed in")
		assert.Equal(123, k.status, "error should have the status that was passed in")
		assert.Equal(nil, k.err, "error should have the err that was passed in")
	}

	{
		usererr := NewErrorUser("test user err", 123, nil)
		assert.Error(usererr, "should not return an empty error")
		err := NewError("", 0, usererr)
		assert.Error(err, "should not return an empty error")
		k := err.(*goverror)
		assert.Equal("test user err", k.message, "error should have its nearest causer message by default")
		assert.Equal(123, k.status, "error should have its nearest causer status by default")
		assert.Equal(usererr, k.err, "error should have the err that was passed in")
	}
}

func TestGoverror_Error(t *testing.T) {
	assert := assert.New(t)

	{
		rootErr := errors.New("test root err")
		err := NewError("test message", 123, rootErr)
		err2 := NewError("test message 2", 0, err)
		assert.Equal("test message 2: test message: test root err", err2.Error(), "error string should have the messages of its causers")
	}

	{
		err := NewError("test message", 123, nil)
		assert.Equal("test message", err.Error(), "error string should its message if there are no causers")
	}
}

func TestGoverror_Unwrap(t *testing.T) {
	assert := assert.New(t)

	rootErr := errors.New("test root err")
	err := NewError("test message", 123, rootErr)
	k := err.(interface {
		Unwrap() error
	})
	assert.Equal(rootErr, k.Unwrap(), "error should return its direct causer when unwrapped")
}

func TestGoverror_Is(t *testing.T) {
	assert := assert.New(t)

	err := NewError("test message", 123, nil)
	goverr := &goverror{}
	ok := xerrors.Is(err, goverr)
	assert.True(ok, "error should be a goverror")
}

func TestGoverror_As(t *testing.T) {
	assert := assert.New(t)

	rootErr := errors.New("test root err")
	err := NewError("test message", 123, rootErr)
	goverr := &goverror{}
	k := err.(interface {
		As(target interface{}) bool
	})
	ok := k.As(goverr)
	assert.True(ok, "error should be a goverror")
	assert.Equal("test message", goverr.message, "error.As should copy message")
	assert.Equal(123, goverr.status, "error.As should copy status")
	assert.Equal(rootErr, goverr.err, "error.As should copy err")
	goverruser := &goverrorUser{}
	ok1 := k.As(goverruser)
	assert.False(ok1, "goverror should not be a goverrorUser by default")

	goverr2 := &goverror{}
	ok2 := xerrors.As(err, &goverr2)
	assert.True(ok2, "error should be a goverror")
	assert.Equal("test message", goverr2.message, "error.As should copy message")
	assert.Equal(123, goverr2.status, "error.As should copy status")
	assert.Equal(rootErr, goverr2.err, "error.As should copy err")
}

func TestNewErrorUser(t *testing.T) {
	assert := assert.New(t)

	rootErr := errors.New("test root err")

	{
		err := NewErrorUser("test message", 123, rootErr)
		assert.Error(err, "should not return an empty error")
		k := err.(*goverrorUser)
		assert.Equal("test message", k.message, "error should have the message that was passed in")
		assert.Equal(123, k.status, "error should have the status that was passed in")
		assert.Equal(rootErr, k.err, "error should have the err that was passed in")

		err2 := NewErrorUser("", 0, err)
		assert.Error(err2, "should not return an empty error")
		k2 := err2.(*goverrorUser)
		assert.Equal("test message", k2.message, "error should have its nearest goverrorUser causer message by default")
		assert.Equal(123, k2.status, "error should have its nearest goverrorUser causer status by default")
		assert.Equal(err, k2.err, "error should have the err that was passed in")
	}

	{
		err := NewErrorUser("", 0, rootErr)
		assert.Error(err, "should not return an empty error")
		k := err.(*goverrorUser)
		assert.Equal("test root err", k.message, "error should have its nearest causer message if no goverrorUser by default")
		assert.Equal(0, k.status, "error should have its nearest goverrorUser causer status if no goverrorUser by default")
		assert.Equal(rootErr, k.err, "error should have the err that was passed in")
	}

	{
		err := NewErrorUser("test message", 123, nil)
		assert.Error(err, "should not return an empty error")
		k := err.(*goverrorUser)
		assert.Equal("test message", k.message, "error should have the message that was passed in")
		assert.Equal(123, k.status, "error should have the status that was passed in")
		assert.Equal(nil, k.err, "error should have the err that was passed in")
	}

	{
		goverr := NewError("test user err", 123, nil)
		assert.Error(goverr, "should not return an empty error")
		err := NewErrorUser("", 0, goverr)
		assert.Error(err, "should not return an empty error")
		k := err.(*goverrorUser)
		assert.Equal("test user err", k.message, "error should have its nearest causer message by default")
		assert.Equal(123, k.status, "error should have its nearest causer status by default")
		assert.Equal(goverr, k.err, "error should have the err that was passed in")
	}
}

func TestGoverrorUser_Error(t *testing.T) {
	assert := assert.New(t)

	{
		rootErr := errors.New("test root err")
		err := NewErrorUser("test message", 123, rootErr)
		err2 := NewErrorUser("test message 2", 0, err)
		assert.Equal("test message 2: test message: test root err", err2.Error(), "error string should have the messages of its causers")
	}

	{
		err := NewErrorUser("test message", 123, nil)
		assert.Equal("test message", err.Error(), "error string should its message if there are no causers")
	}
}

func TestGoverrorUser_Unwrap(t *testing.T) {
	assert := assert.New(t)

	rootErr := errors.New("test root err")
	err := NewErrorUser("test message", 123, rootErr)
	k := err.(interface {
		Unwrap() error
	})
	assert.Equal(rootErr, k.Unwrap(), "error should return its direct causer when unwrapped")
}

func TestGoverrorUser_Is(t *testing.T) {
	assert := assert.New(t)

	err := NewErrorUser("test message", 123, nil)
	goverruser := &goverrorUser{}
	ok := xerrors.Is(err, goverruser)
	assert.True(ok, "error should be a goverrorUser")
}

func TestGoverrorUser_As(t *testing.T) {
	assert := assert.New(t)

	rootErr := errors.New("test root err")
	err := NewErrorUser("test message", 123, rootErr)
	goverruser := &goverrorUser{}
	k := err.(interface {
		As(target interface{}) bool
	})
	ok := k.As(goverruser)
	assert.True(ok, "error should be a goverror")
	assert.Equal("test message", goverruser.message, "error.As should copy message")
	assert.Equal(123, goverruser.status, "error.As should copy status")
	assert.Equal(rootErr, goverruser.err, "error.As should copy err")
	goverr := &goverror{}
	ok1 := k.As(goverr)
	assert.False(ok1, "goverror should not be a goverror by default")

	goverr2 := &goverrorUser{}
	ok2 := xerrors.As(err, &goverr2)
	assert.True(ok2, "error should be a goverror")
	assert.Equal("test message", goverr2.message, "error.As should copy message")
	assert.Equal(123, goverr2.status, "error.As should copy status")
	assert.Equal(rootErr, goverr2.err, "error.As should copy err")
}

func TestErrorStatus(t *testing.T) {
	assert := assert.New(t)

	rootErr := errors.New("test root err")
	err := NewError("test message", 123, rootErr)
	errUser := NewErrorUser("test message user", 234, err)
	errUser2 := NewErrorUser("test message user", 234, rootErr)

	assert.Equal(123, ErrorStatus(err), "error status should be the goverror status")
	assert.Equal(123, ErrorStatus(errUser), "error status should be the goverror status if available")
	assert.Equal(234, ErrorStatus(errUser2), "error status should be the goverrorUser status if no goverror")
	assert.Equal(0, ErrorStatus(rootErr), "error status should be 0 for non goverror errors")
}

func TestErrorHandler(t *testing.T) {
	assert := assert.New(t)

	logbuf := bytes.Buffer{}
	config := Config{
		LogLevel:  envToLevel("INFO"),
		LogOutput: &logbuf,
	}
	l := newLogger(config)
	i := echo.New()
	handler := errorHandler(i, l)

	{
		pathurl := "/error"
		reqbody := `{"ping":"pong"}`
		req := httptest.NewRequest(http.MethodPost, pathurl, strings.NewReader(reqbody))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := i.NewContext(req, rec)
		c.SetPath(pathurl)
		rootErr := errors.New("test root error")
		err := NewError("test error message", http.StatusInternalServerError, rootErr)
		handler(err, c)
		assert.Equal(http.StatusInternalServerError, rec.Code, "http status should be the status of the error")
		assert.Equal(`{"message":"test error message"}`, strings.TrimSpace(rec.Body.String()), "json message should be the message of the error")
	}

	{
		pathurl := "/error"
		reqbody := `{"ping":"pong"}`
		req := httptest.NewRequest(http.MethodPost, pathurl, strings.NewReader(reqbody))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := i.NewContext(req, rec)
		c.SetPath(pathurl)
		err := NewErrorUser("test error message", http.StatusInternalServerError, nil)
		handler(err, c)
		assert.Equal(http.StatusInternalServerError, rec.Code, "http status should be the status of the error")
		assert.Equal(`{"message":"test error message"}`, strings.TrimSpace(rec.Body.String()), "json message should be the message of the error")
	}

	{
		pathurl := "/error"
		reqbody := `{"ping":"pong"}`
		req := httptest.NewRequest(http.MethodPost, pathurl, strings.NewReader(reqbody))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := i.NewContext(req, rec)
		c.SetPath(pathurl)
		echoerr := echo.NewHTTPError(http.StatusNotFound, "Route does not exist")
		handler(echoerr, c)
		assert.Equal(http.StatusNotFound, rec.Code, "http status should be the status of the error")
		assert.Equal(`{"message":"Route does not exist"}`, strings.TrimSpace(rec.Body.String()), "json message should be the message of the error")
	}

	logjson := map[string]interface{}{}
	err := json.Unmarshal(logbuf.Bytes(), &logjson)
	assert.NoError(err, "log output must be json format")
	assert.Equal("/error", logjson["endpoint"], "endpoint must be set in log")
	assert.Equal("error", logjson["level"], "level must be set in log")
	assert.Equal("test error message: test root error", logjson["msg"], "full error message must be set in log")
	assert.NotEqual("", logjson["request"], "full request must be set in log")
	ti := time.Time{}
	errtime := ti.UnmarshalText([]byte(logjson["logtime"].(string)))
	assert.NoError(errtime, "full request time must be set in log")
}
