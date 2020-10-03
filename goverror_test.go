package governor

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/assert"
	"mime"
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
		assert.Equal(false, k.noLog, "error should be logged")

		err2 := NewError("", 0, err)
		assert.Error(err2, "should not return an empty error")
		k2 := err2.(*goverror)
		assert.Equal("test message", k2.message, "error should have its nearest goverror causer message by default")
		assert.Equal(123, k2.status, "error should have its nearest goverror causer status by default")
		assert.Equal(err, k2.err, "error should have the err that was passed in")
		assert.Equal(false, k2.noLog, "error should be logged")
	}

	{
		err := NewError("", 0, rootErr)
		assert.Error(err, "should not return an empty error")
		k := err.(*goverror)
		assert.Equal("test root err", k.message, "error should have its nearest causer message if no goverror by default")
		assert.Equal(0, k.status, "error should have its nearest goverror causer status if no goverror by default")
		assert.Equal(rootErr, k.err, "error should have the err that was passed in")
		assert.Equal(false, k.noLog, "error should be logged")
	}

	{
		err := NewError("test message", 123, nil)
		assert.Error(err, "should not return an empty error")
		k := err.(*goverror)
		assert.Equal("test message", k.message, "error should have the message that was passed in")
		assert.Equal(123, k.status, "error should have the status that was passed in")
		assert.Equal(nil, k.err, "error should have the err that was passed in")
		assert.Equal(false, k.noLog, "error should be logged")
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
		assert.Equal(false, k.noLog, "error should be logged")
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
	ok := errors.Is(err, goverr)
	assert.True(ok, "error should be a goverror")
}

func TestGoverror_As(t *testing.T) {
	assert := assert.New(t)

	rootErr := errors.New("test root err")
	{
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
		assert.Equal(false, goverr.noLog, "error.As should copy noLog")

		goverr2 := &goverror{}
		ok2 := errors.As(err, &goverr2)
		assert.True(ok2, "error should be a goverror")
		assert.Equal("test message", goverr2.message, "error.As should copy message")
		assert.Equal(123, goverr2.status, "error.As should copy status")
		assert.Equal(rootErr, goverr2.err, "error.As should copy err")
		assert.Equal(false, goverr.noLog, "error.As should copy noLog")
	}

	{
		err := NewErrorUser("test message", 123, rootErr)
		goverr := &goverror{}
		ok := errors.As(err, &goverr)
		assert.True(ok, "error should be a goverror")
		assert.Equal("test message", goverr.message, "error.As should copy message")
		assert.Equal(123, goverr.status, "error.As should copy status")
		assert.Equal(rootErr, goverr.err, "error.As should copy err")
		assert.Equal(true, goverr.noLog, "error.As should copy noLog")
	}
}

func TestNewErrorUser(t *testing.T) {
	assert := assert.New(t)

	rootErr := errors.New("test root err")

	{
		err := NewErrorUser("test message", 123, rootErr)
		assert.Error(err, "should not return an empty error")
		k := err.(*goverror)
		assert.Equal("test message", k.message, "error should have the message that was passed in")
		assert.Equal(123, k.status, "error should have the status that was passed in")
		assert.Equal(rootErr, k.err, "error should have the err that was passed in")
		assert.Equal(true, k.noLog, "error should not be logged")

		err2 := NewErrorUser("", 0, err)
		assert.Error(err2, "should not return an empty error")
		k2 := err2.(*goverror)
		assert.Equal("test message", k2.message, "error should have its nearest goverrorUser causer message by default")
		assert.Equal(123, k2.status, "error should have its nearest goverrorUser causer status by default")
		assert.Equal(err, k2.err, "error should have the err that was passed in")
		assert.Equal(true, k.noLog, "error should not be logged")
	}

	{
		err := NewErrorUser("", 0, rootErr)
		assert.Error(err, "should not return an empty error")
		k := err.(*goverror)
		assert.Equal("test root err", k.message, "error should have its nearest causer message if no goverrorUser by default")
		assert.Equal(0, k.status, "error should have its nearest goverrorUser causer status if no goverrorUser by default")
		assert.Equal(rootErr, k.err, "error should have the err that was passed in")
		assert.Equal(true, k.noLog, "error should not be logged")
	}

	{
		err := NewErrorUser("test message", 123, nil)
		assert.Error(err, "should not return an empty error")
		k := err.(*goverror)
		assert.Equal("test message", k.message, "error should have the message that was passed in")
		assert.Equal(123, k.status, "error should have the status that was passed in")
		assert.Equal(nil, k.err, "error should have the err that was passed in")
		assert.Equal(true, k.noLog, "error should not be logged")
	}

	{
		goverr := NewError("test user err", 123, nil)
		assert.Error(goverr, "should not return an empty error")
		err := NewErrorUser("", 0, goverr)
		assert.Error(err, "should not return an empty error")
		k := err.(*goverror)
		assert.Equal("test user err", k.message, "error should have its nearest causer message by default")
		assert.Equal(123, k.status, "error should have its nearest causer status by default")
		assert.Equal(goverr, k.err, "error should have the err that was passed in")
		assert.Equal(true, k.noLog, "error should not be logged")
	}
}

func TestErrorStatus(t *testing.T) {
	assert := assert.New(t)

	rootErr := errors.New("test root err")
	err := NewError("test message", 123, rootErr)
	errUser := NewErrorUser("test message user", 234, err)
	errUser2 := NewErrorUser("test message user", 234, rootErr)

	assert.Equal(123, ErrorStatus(err), "error status should be the goverror status")
	assert.Equal(234, ErrorStatus(errUser), "error status should be the goverror status if available")
	assert.Equal(234, ErrorStatus(errUser2), "error status should be the goverrorUser status if no goverror")
	assert.Equal(0, ErrorStatus(rootErr), "error status should be 0 for non goverror errors")
}

func TestContextWriteError(t *testing.T) {
	assert := assert.New(t)

	logbuf1 := &bytes.Buffer{}
	l1 := newLogger(Config{
		logLevel:  envToLevel("INFO"),
		logOutput: logbuf1,
	})

	{
		pathurl := "/error"
		reqbody := `{"ping":"pong"}`
		req := httptest.NewRequest(http.MethodPost, pathurl, strings.NewReader(reqbody))
		req.Header.Set("Content-Type", mime.FormatMediaType("application/json", map[string]string{"charset": "utf-8"}))
		rec := httptest.NewRecorder()
		c := NewContext(rec, req, l1)
		rootErr := errors.New("test root error")
		err := NewError("test error message", http.StatusInternalServerError, rootErr)
		c.WriteError(err)
		assert.Equal(http.StatusInternalServerError, rec.Code, "http status should be the status of the error")
		assert.Equal(`{"message":"test error message"}`, strings.TrimSpace(rec.Body.String()), "json message should be the message of the error")
	}

	{
		pathurl := "/error"
		reqbody := `{"ping":"pong"}`
		req := httptest.NewRequest(http.MethodPost, pathurl, strings.NewReader(reqbody))
		req.Header.Set("Content-Type", mime.FormatMediaType("application/json", map[string]string{"charset": "utf-8"}))
		rec := httptest.NewRecorder()
		c := NewContext(rec, req, l1)
		err := NewErrorUser("test error message", http.StatusInternalServerError, nil)
		c.WriteError(err)
		assert.Equal(http.StatusInternalServerError, rec.Code, "http status should be the status of the error")
		assert.Equal(`{"message":"test error message"}`, strings.TrimSpace(rec.Body.String()), "json message should be the message of the error")
	}

	{
		logjson := map[string]interface{}{}
		err := json.Unmarshal(logbuf1.Bytes(), &logjson)
		assert.NoError(err, "log output must be json format")
		assert.Equal("/error", logjson["endpoint"], "endpoint must be set in log")
		assert.Equal("error", logjson["level"], "level must be set in log")
		assert.Equal("test error message", logjson["msg"], "full error message must be set in log")
		assert.Equal("test error message: test root error", logjson["error"], "full error trace must be set in log")
		assert.NotEqual("", logjson["request"], "full request must be set in log")
		ti := time.Time{}
		errtime := ti.UnmarshalText([]byte(logjson["time"].(string)))
		assert.NoError(errtime, "full request time must be set in log")
	}

	logbuf2 := &bytes.Buffer{}
	l2 := newLogger(Config{
		logLevel:  envToLevel("INFO"),
		logOutput: logbuf2,
	})

	{
		pathurl := "/error-plain"
		reqbody := `{"ping":"pong"}`
		req := httptest.NewRequest(http.MethodPost, pathurl, strings.NewReader(reqbody))
		req.Header.Set("Content-Type", mime.FormatMediaType("application/json", map[string]string{"charset": "utf-8"}))
		rec := httptest.NewRecorder()
		c := NewContext(rec, req, l2)
		err := errors.New("Plain error")
		c.WriteError(err)
		assert.Equal(http.StatusInternalServerError, rec.Code, "http status should be the status of the error")
		assert.Equal(`{"message":"Internal Server Error"}`, strings.TrimSpace(rec.Body.String()), "json message should be the message of the error")
	}

	{
		logjson := map[string]interface{}{}
		err := json.Unmarshal(logbuf2.Bytes(), &logjson)
		assert.NoError(err, "log output must be json format")
		assert.Equal("/error-plain", logjson["endpoint"], "endpoint must be set in log")
		assert.Equal("error", logjson["level"], "level must be set in log")
		assert.Equal("non governor error", logjson["msg"], "error message must be set in log")
		assert.Equal("Plain error", logjson["error"], "actual error message must be set in log")
		assert.NotEqual("", logjson["request"], "full request must be set in log")
		ti := time.Time{}
		errtime := ti.UnmarshalText([]byte(logjson["time"].(string)))
		assert.NoError(errtime, "full request time must be set in log")
	}
}
