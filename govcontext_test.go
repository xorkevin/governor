package governor

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"xorkevin.dev/kerrors"
)

type (
	testErr struct{}
)

func (e testErr) Error() string {
	return "test struct err"
}

func TestContext(t *testing.T) {
	t.Parallel()

	stackRegex := regexp.MustCompile(`Stack trace \[\S+ \S+:\d+\]`)
	fullStackRegex := regexp.MustCompile(`^(?:\S+\n\t\S+:\d+ \(0x[0-9a-f]+\)\n)+$`)

	t.Run("WriteError", func(t *testing.T) {
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
				Level:    "ERROR",
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
				Level:    "WARN",
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
				Level:    "ERROR",
				LogMsg:   "plain-error",
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

				logbuf := bytes.Buffer{}
				l := newLogger(Config{
					logger: configLogger{
						level:  "INFO",
						output: "TEST",
						writer: &logbuf,
					},
				})
				req := httptest.NewRequest(http.MethodPost, tc.Path, strings.NewReader(tc.Body))
				req.Header.Set("Content-Type", mime.FormatMediaType("application/json", map[string]string{"charset": "utf-8"}))
				rec := httptest.NewRecorder()
				c := NewContext(rec, req, l.Logger)
				c.WriteError(tc.Err)
				assert.Equal(tc.Status, rec.Code)
				assert.Equal(tc.Res, strings.TrimSpace(rec.Body.String()))
				if tc.NoLog {
					assert.Equal(0, logbuf.Len())
					return
				}

				var j struct {
					Level      string `json:"level"`
					UnixtimeUS int64  `json:"unixtimeus"`
					Msg        string `json:"msg"`
					Error      string `json:"error"`
					StackTrace string `json:"stacktrace"`
				}
				d := json.NewDecoder(&logbuf)
				assert.NoError(d.Decode(&j))
				assert.Equal(tc.Level, j.Level)
				assert.True(j.UnixtimeUS > 0)
				assert.Equal(tc.LogMsg, j.Msg)
				if tc.NoTrace {
					assert.Equal(tc.LogError, j.Error)
					assert.Equal("NONE", j.StackTrace)
				} else {
					assert.Regexp(stackRegex, j.Error)
					assert.Equal(tc.LogError, stackRegex.ReplaceAllString(j.Error, "%!(STACKTRACE)"))
					assert.Regexp(fullStackRegex, j.StackTrace)
				}
				assert.False(d.More())
			})
		}
	})
}
