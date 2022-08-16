package governor

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"net/http/httptest"
	"strconv"
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
		t.Parallel()

		errorsErr := errors.New("test errors err")
		govErr := ErrWithRes(errorsErr, http.StatusInternalServerError, "test_gov_err_code", "test gov error")

		for _, tc := range []struct {
			Test   string
			Opts   []ErrorOpt
			Msg    string
			Kind   error
			Res    *ErrorRes
			ErrMsg string
		}{
			{
				Test:   "produces an error with an errors kind and message",
				Opts:   []ErrorOpt{ErrOptMsg("test message 123"), ErrOptKind(errorsErr)},
				Msg:    "test message 123",
				Kind:   errorsErr,
				ErrMsg: "test message 123 [test errors err]: Stack trace [Error stack trace]",
			},
			{
				Test:   "produces an error with an error struct kind and message",
				Opts:   []ErrorOpt{ErrOptMsg("test message 321"), ErrOptKind(testErr{})},
				Msg:    "test message 321",
				Kind:   testErr{},
				ErrMsg: "test message 321 [test struct err]: Stack trace [Error stack trace]",
			},
			{
				Test: "produces an error with an error res",
				Opts: []ErrorOpt{ErrOptMsg("test message 456"), ErrOptKind(&ErrorRes{
					Status:  http.StatusBadRequest,
					Code:    "test_err_code",
					Message: "test response message 456",
				})},
				Msg: "test message 456",
				Res: &ErrorRes{
					Status:  http.StatusBadRequest,
					Code:    "test_err_code",
					Message: "test response message 456",
				},
				ErrMsg: "test message 456 [400 [test_err_code]: test response message 456]: Stack trace [Error stack trace]",
			},
			{
				Test: "produces an error with a deeply nested error",
				Opts: []ErrorOpt{ErrOptMsg("test message 654"), ErrOptKind(&ErrorRes{
					Status:  http.StatusBadRequest,
					Code:    "test_err_code",
					Message: "test response message 654",
				}), ErrOptInner(govErr)},
				Msg: "test message 654",
				Res: &ErrorRes{
					Status:  http.StatusBadRequest,
					Code:    "test_err_code",
					Message: "test response message 654",
				},
				ErrMsg: "test message 654 [400 [test_err_code]: test response message 654]: Error response [500 [test_gov_err_code]: test gov error]: Stack trace [Error stack trace]: test errors err",
			},
		} {
			tc := tc
			t.Run(tc.Test, func(t *testing.T) {
				t.Parallel()

				assert := require.New(t)

				err := NewError(tc.Opts...)
				assert.Error(err)
				var k *Error
				assert.ErrorAs(err, &k)
				assert.Equal(tc.Msg, k.Message)
				assert.Equal(tc.ErrMsg, err.Error())
				if tc.Kind != nil {
					assert.ErrorIs(err, tc.Kind)
				}
				if tc.Res != nil {
					var r *ErrorRes
					assert.ErrorAs(err, &r)
					assert.Equal(tc.Res, r)
				}
			})
		}
	})

	t.Run("Context.WriteError", func(t *testing.T) {
		t.Parallel()

		rootErr := errors.New("test root error")
		govErr := ErrWithRes(rootErr, http.StatusBadRequest, "test_gov_err_code", "test gov error")

		for _, tc := range []struct {
			Test   string
			Err    error
			Path   string
			Body   string
			Status int
			Res    string
			Level  string
			LogMsg string
			Log    string
			NoLog  bool
		}{
			{
				Test: "logs the error",
				Err: NewError(ErrOptMsg("test error message"), ErrOptKind(&ErrorRes{
					Status:  http.StatusInternalServerError,
					Code:    "err_code_890",
					Message: "test error response message",
				}), ErrOptInner(rootErr)),
				Path:   "/error1",
				Body:   `{"ping":"pong"}`,
				Status: http.StatusInternalServerError,
				Res:    `{"code":"err_code_890","message":"test error response message"}`,
				Level:  "error",
				LogMsg: "test error message",
				Log:    "test error message [500 [err_code_890]: test error response message]: Stack trace [Error stack trace]: test root error",
			},
			{
				Test:   "sends the Error with a non zero status",
				Err:    NewError(ErrOptMsg("test error message"), ErrOptKind(testErr{}), ErrOptInner(govErr)),
				Path:   "/error9",
				Body:   `{"ping":"pong"}`,
				Status: http.StatusBadRequest,
				Res:    `{"code":"test_gov_err_code","message":"test gov error"}`,
				Level:  "warn",
				LogMsg: "test error message",
				Log:    "test error message [test struct err]: Error response [400 [test_gov_err_code]: test gov error]: Stack trace [Error stack trace]: test root error",
			},
			{
				Test:   "can send arbitrary errors",
				Err:    errors.New("Plain error"),
				Path:   "/error2",
				Body:   `{"ping":"pong"}`,
				Status: http.StatusInternalServerError,
				Res:    `{"message":"Internal Server Error"}`,
				Level:  "error",
				LogMsg: "non governor error",
				Log:    "Plain error",
			},
		} {
			tc := tc
			t.Run(tc.Test, func(t *testing.T) {
				t.Parallel()

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
					UnixTime string `json:"unixtime"`
				}{}
				assert.NoError(json.Unmarshal(logbuf.Bytes(), &logjson))
				assert.Equal(tc.Level, logjson.Level)
				assert.Equal("root", logjson.Module)
				assert.Equal(tc.Path, logjson.Endpoint)
				assert.Equal(tc.LogMsg, logjson.Msg)
				assert.Equal(tc.Log, logjson.Error)
				ti, err := time.Parse(time.RFC3339, logjson.Time)
				assert.NoError(err)
				assert.True(ti.After(time.Unix(0, 0)))
				ut, err := strconv.ParseInt(logjson.UnixTime, 10, 64)
				assert.NoError(err)
				assert.True(time.Unix(ut, 0).After(time.Unix(0, 0)))
			})
		}
	})
}
