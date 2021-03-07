package governor

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type (
	testErr struct{}
)

func (e testErr) Error() string {
	return "test struct err"
}

func TestError(t *testing.T) {
	t.Run("NewError", func(t *testing.T) {
		errorsErr := errors.New("test errors err")
		govErr := NewError(ErrOptRes(ErrorRes{
			Status:  http.StatusInternalServerError,
			Code:    "test_gov_err_code",
			Message: "test gov error",
		}), ErrOptInner(errorsErr))

		for _, tc := range []struct {
			Test   string
			Opts   []ErrorOpt
			Status int
			Code   string
			Msg    string
			Kind   error
			ErrMsg string
		}{
			{
				Test:   "produces an error with an errors kind and message",
				Opts:   []ErrorOpt{ErrOptKind(errorsErr), ErrOptMsg("test message 123")},
				Status: 0,
				Code:   "",
				Msg:    "test message 123",
				Kind:   errorsErr,
				ErrMsg: "[test errors err] test message 123",
			},
			{
				Test:   "produces an error with an error struct kind and message",
				Opts:   []ErrorOpt{ErrOptKind(testErr{}), ErrOptMsg("test message 321")},
				Status: 0,
				Code:   "",
				Msg:    "test message 321",
				Kind:   testErr{},
				ErrMsg: "[test struct err] test message 321",
			},
			{
				Test: "produces an error with an error res",
				Opts: []ErrorOpt{ErrOptKind(testErr{}), ErrOptRes(ErrorRes{
					Status:  http.StatusBadRequest,
					Code:    "test_err_code",
					Message: "test message 456",
				})},
				Status: http.StatusBadRequest,
				Code:   "test_err_code",
				Msg:    "test message 456",
				Kind:   testErr{},
				ErrMsg: "[test struct err] test_err_code test message 456",
			},
			{
				Test: "produces an error with a deeply nested error",
				Opts: []ErrorOpt{ErrOptKind(testErr{}), ErrOptRes(ErrorRes{
					Status:  http.StatusBadRequest,
					Code:    "test_err_code",
					Message: "test message 654",
				}), ErrOptInner(govErr)},
				Status: http.StatusBadRequest,
				Code:   "test_err_code",
				Msg:    "test message 654",
				Kind:   errorsErr,
				ErrMsg: "[test struct err] test_err_code test message 654: test_gov_err_code test gov error: test errors err",
			},
		} {
			t.Run(tc.Test, func(t *testing.T) {
				assert := require.New(t)

				err := NewError(tc.Opts...)
				assert.Error(err)
				k := &Error{}
				assert.ErrorAs(err, k)
				assert.Equal(tc.Status, k.Status)
				assert.Equal(tc.Code, k.Code)
				assert.Equal(tc.Msg, k.Message)
				assert.ErrorIs(err, tc.Kind)
				assert.Equal(tc.ErrMsg, err.Error())
			})
		}
	})

	t.Run("Context.WriteError", func(t *testing.T) {
		rootErr := errors.New("test root error")
		govErr := NewError(ErrOptRes(ErrorRes{
			Status:  http.StatusBadRequest,
			Code:    "test_gov_err_code",
			Message: "test gov error",
		}), ErrOptInner(rootErr))

		for _, tc := range []struct {
			Test   string
			Err    error
			Path   string
			Body   string
			Status int
			Res    string
			LogMsg string
			Log    string
			NoLog  bool
		}{
			{
				Test: "logs the error",
				Err: NewError(ErrOptKind(testErr{}), ErrOptRes(ErrorRes{
					Status:  http.StatusInternalServerError,
					Code:    "err_code_890",
					Message: "test error message",
				}), ErrOptInner(rootErr)),
				Path:   "/error1",
				Body:   `{"ping":"pong"}`,
				Status: http.StatusInternalServerError,
				Res:    `{"code":"err_code_890","message":"test error message"}`,
				LogMsg: "test error message",
				Log:    "[test struct err] err_code_890 test error message: test root error",
			},
			{
				Test:   "sends the Error with a non zero status",
				Err:    NewError(ErrOptKind(testErr{}), ErrOptMsg("test error message"), ErrOptInner(govErr)),
				Path:   "/error9",
				Body:   `{"ping":"pong"}`,
				Status: http.StatusBadRequest,
				Res:    `{"code":"test_gov_err_code","message":"test gov error"}`,
				LogMsg: "test error message",
				Log:    "[test struct err] test error message: test_gov_err_code test gov error: test root error",
			},
			{
				Test:   "can send arbitrary errors",
				Err:    errors.New("Plain error"),
				Path:   "/error2",
				Body:   `{"ping":"pong"}`,
				Status: http.StatusInternalServerError,
				Res:    `{"message":"Internal Server Error"}`,
				LogMsg: "non governor error",
				Log:    "Plain error",
			},
			{
				Test: "does not log user errors",
				Err: NewError(ErrOptUser, ErrOptRes(ErrorRes{
					Status:  http.StatusBadRequest,
					Code:    "test_err_code",
					Message: "an error message",
				}), ErrOptInner(rootErr)),
				Path:   "/error8",
				Body:   `{"ping":"pong"}`,
				Status: http.StatusBadRequest,
				Res:    `{"code":"test_err_code","message":"an error message"}`,
				NoLog:  true,
			},
		} {
			t.Run(tc.Test, func(t *testing.T) {
				assert := require.New(t)
				logbuf := &bytes.Buffer{}
				l := newLogger(Config{
					logLevel:  envToLevel("INFO"),
					logOutput: logbuf,
				})
				req := httptest.NewRequest(http.MethodPost, tc.Path, bytes.NewReader([]byte(tc.Body)))
				req.Header.Set("Content-Type", mime.FormatMediaType("application/json", map[string]string{"charset": "utf-8"}))
				rec := httptest.NewRecorder()
				c := NewContext(rec, req, l)
				c.WriteError(tc.Err)
				assert.Equal(tc.Status, rec.Code)
				assert.Equal(tc.Res, strings.TrimSpace(rec.Body.String()))
				if tc.NoLog {
					assert.Equal(0, logbuf.Len())
					return
				}

				logjson := struct {
					Level    string `json:"level"`
					Module   string `json:"module"`
					Endpoint string `json:"endpoint"`
					Msg      string `json:"msg"`
					Error    string `json:"error"`
					Time     string `json:"time"`
					UnixTime int64  `json:"unixtime"`
				}{}
				assert.NoError(json.Unmarshal(logbuf.Bytes(), &logjson))
				assert.Equal("error", logjson.Level)
				assert.Equal("root", logjson.Module)
				assert.Equal(tc.Path, logjson.Endpoint)
				assert.Equal(tc.LogMsg, logjson.Msg)
				assert.Equal(tc.Log, logjson.Error)
				ti, err := time.Parse(time.RFC3339, logjson.Time)
				assert.NoError(err)
				assert.True(ti.After(time.Unix(0, 0)))
				assert.True(time.Unix(logjson.UnixTime, 0).After(time.Unix(0, 0)))
			})
		}
	})
}
