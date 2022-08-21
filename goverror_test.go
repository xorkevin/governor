package governor

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"xorkevin.dev/kerrors"
)

type (
	testErr struct{}
)

func (e testErr) Error() string {
	return "test struct err"
}

func TestError(t *testing.T) {
	t.Parallel()

	stackRegex := regexp.MustCompile(`Stack trace \[\S+ \S+:\d+\]`)
	fullStackRegex := regexp.MustCompile(`^(?:\S+\n\t\S+:\d+ \(0x[0-9a-f]+\)\n)+$`)

	t.Run("Context.WriteError", func(t *testing.T) {
		t.Parallel()

		for _, tc := range []struct {
			Test     string
			Err      error
			Path     string
			Body     string
			Status   int
			Res      string
			Level    string
			LogMsg   string
			LogError string
			NoTrace  bool
			NoLog    bool
		}{
			{
				Test:     "logs the error",
				Err:      ErrWithRes(errors.New("test root error"), http.StatusInternalServerError, "err_code_890", "test error response message"),
				Path:     "/error1",
				Body:     `{"ping":"pong"}`,
				Status:   http.StatusInternalServerError,
				Res:      `{"code":"err_code_890","message":"test error response message"}`,
				Level:    "error",
				LogMsg:   "Error response",
				LogError: "Error response [(500) test error response message [err_code_890]]: %!(STACKTRACE): test root error",
			},
			{
				Test:     "sends the nested error with a non zero status",
				Err:      kerrors.WithMsg(ErrWithRes(testErr{}, http.StatusBadRequest, "test_err_code", "test error"), "some message"),
				Path:     "/error9",
				Body:     `{"ping":"pong"}`,
				Status:   http.StatusBadRequest,
				Res:      `{"code":"test_err_code","message":"test error"}`,
				Level:    "warn",
				LogMsg:   "some message",
				LogError: "some message: Error response [(400) test error [test_err_code]]: %!(STACKTRACE): test struct err",
			},
			{
				Test:     "can send arbitrary errors",
				Err:      errors.New("Plain error"),
				Path:     "/error2",
				Body:     `{"ping":"pong"}`,
				Status:   http.StatusInternalServerError,
				Res:      `{"message":"Internal Server Error"}`,
				Level:    "error",
				LogMsg:   "non-kerror",
				LogError: "Plain error",
				NoTrace:  true,
			},
			{
				Test:   "respects ErrorNoLog",
				Err:    ErrWithRes(ErrWithNoLog(errors.New("test root error")), http.StatusInternalServerError, "some_err_code", "test err message"),
				Path:   "/error8",
				Body:   `{"ping":"pong"}`,
				Status: http.StatusInternalServerError,
				Res:    `{"code":"some_err_code","message":"test err message"}`,
				NoLog:  true,
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
				req := httptest.NewRequest(http.MethodPost, tc.Path, strings.NewReader(tc.Body))
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
					Level      string `json:"level"`
					Module     string `json:"module"`
					Endpoint   string `json:"endpoint"`
					Msg        string `json:"msg"`
					Error      string `json:"error"`
					StackTrace string `json:"stacktrace"`
					Time       string `json:"time"`
					UnixTime   string `json:"unixtime"`
				}{}
				assert.NoError(json.Unmarshal(logbuf.Bytes(), &logjson))
				assert.Equal(tc.Level, logjson.Level)
				assert.Equal("root", logjson.Module)
				assert.Equal(tc.Path, logjson.Endpoint)
				assert.Equal(tc.LogMsg, logjson.Msg)
				if tc.NoTrace {
					assert.Equal(tc.LogError, logjson.Error)
					assert.Equal("NONE", logjson.StackTrace)
				} else {
					assert.Regexp(stackRegex, logjson.Error)
					assert.Equal(tc.LogError, stackRegex.ReplaceAllString(logjson.Error, "%!(STACKTRACE)"))
					assert.Regexp(fullStackRegex, logjson.StackTrace)
				}
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
