package governor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"nhooyr.io/websocket"
	"xorkevin.dev/governor/util/kjson"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	testErr struct{}
)

func (e testErr) Error() string {
	return "test struct err"
}

type (
	threadSafeBuffer struct {
		b bytes.Buffer
		m sync.Mutex
	}
)

func (b *threadSafeBuffer) Read(p []byte) (n int, err error) {
	b.m.Lock()
	defer b.m.Unlock()
	return b.b.Read(p)
}

func (b *threadSafeBuffer) Write(p []byte) (n int, err error) {
	b.m.Lock()
	defer b.m.Unlock()
	return b.b.Write(p)
}

func (b *threadSafeBuffer) String() string {
	b.m.Lock()
	defer b.m.Unlock()
	return b.b.String()
}

func TestContext(t *testing.T) {
	t.Parallel()

	stackRegex := regexp.MustCompile(`Stack trace\n\[\[\n\S+ \S+:\d+\n\]\]`)

	generateTestContext := func(method, path string, body io.Reader) (*http.Request, *httptest.ResponseRecorder, *bytes.Buffer, *Context) {
		logbuf := &bytes.Buffer{}
		log := klog.New(
			klog.OptHandler(klog.NewJSONSlogHandler(klog.NewSyncWriter(logbuf))),
		)
		req := httptest.NewRequest(method, path, body)
		rec := httptest.NewRecorder()
		return req, rec, logbuf, NewContext(rec, req, log)
	}

	t.Run("ReadAllBody", func(t *testing.T) {
		t.Parallel()

		assert := require.New(t)

		_, _, _, c := generateTestContext(http.MethodPost, "/api/path", strings.NewReader("test body contents"))

		body, err := c.ReadAllBody()
		assert.NoError(err)
		assert.Equal([]byte("test body contents"), body)
	})

	t.Run("Bind", func(t *testing.T) {
		t.Parallel()

		for _, tc := range []struct {
			Test         string
			ContentType  string
			Body         string
			AllowUnknown bool
			Value        string
			Error        string
		}{
			{
				Test:         "may allow unknown fields",
				ContentType:  "application/json",
				Body:         `{"ping":"pong","unknown":"value"}`,
				AllowUnknown: true,
				Value:        "pong",
			},
			{
				Test:         "may disallow unknown fields",
				ContentType:  "application/json",
				Body:         `{"ping":"pong","unknown":"value"}`,
				AllowUnknown: false,
				Error:        "Unknown field",
			},
			{
				Test:         "errors on no media type",
				Body:         `{"ping":"pong","unknown":"value"}`,
				AllowUnknown: true,
				Error:        "No media type",
			},
			{
				Test:         "errors on unsupported media type",
				Body:         `{"ping":"pong","unknown":"value"}`,
				ContentType:  "text/plain",
				AllowUnknown: true,
				Error:        "Unsupported media type",
			},
			{
				Test:         "errors on empty body",
				ContentType:  "application/json",
				AllowUnknown: true,
				Error:        "Empty request body",
			},
			{
				Test:         "errors on malformed json",
				ContentType:  "application/json",
				Body:         `{bogus}`,
				AllowUnknown: true,
				Error:        "Invalid JSON",
			},
			{
				Test:         "errors on too much json",
				ContentType:  "application/json",
				Body:         `{"ping":"pong"}{"more":"stuff"}`,
				AllowUnknown: true,
				Error:        "Invalid JSON",
			},
		} {
			t.Run(tc.Test, func(t *testing.T) {
				t.Parallel()

				assert := require.New(t)

				var reqbody io.Reader
				if tc.Body != "" {
					reqbody = strings.NewReader(tc.Body)
				}
				req, _, _, c := generateTestContext(http.MethodPost, "/api/path", reqbody)
				if tc.ContentType != "" {
					req.Header.Set(headerContentType, tc.ContentType)
				}

				var body struct {
					Ping string `json:"ping"`
				}
				err := c.Bind(&body, tc.AllowUnknown)
				if tc.Error == "" {
					assert.NoError(err)
					assert.Equal(tc.Value, body.Ping)
					return
				}
				assert.Error(err)
				assert.Contains(err.Error(), tc.Error)
			})
		}
	})

	t.Run("FormValue", func(t *testing.T) {
		t.Parallel()

		assert := require.New(t)

		req, _, _, c := generateTestContext(http.MethodPost, "/api/path", strings.NewReader("ping=pong&other=value"))
		req.Header.Set(headerContentType, "application/x-www-form-urlencoded")

		assert.Equal("pong", c.FormValue("ping"))
		assert.Equal("value", c.FormValue("other"))
	})

	t.Run("FormFile", func(t *testing.T) {
		t.Parallel()

		assert := require.New(t)

		var reqbody bytes.Buffer
		reqwriter := multipart.NewWriter(&reqbody)
		reqfilewriter, err := reqwriter.CreateFormFile("testfile", "testfilename")
		assert.NoError(err)
		_, err = io.Copy(reqfilewriter, strings.NewReader("test form file"))
		assert.NoError(err)
		assert.NoError(reqwriter.Close())
		req, _, _, c := generateTestContext(http.MethodPost, "/api/path", &reqbody)
		req.Header.Set(headerContentType, reqwriter.FormDataContentType())

		func() {
			file, header, err := c.FormFile("testfile")
			assert.NoError(err)
			defer func() {
				assert.NoError(file.Close())
			}()
			assert.Equal("testfilename", header.Filename)
			body, err := io.ReadAll(file)
			assert.NoError(err)
			assert.Equal("test form file", string(body))
		}()
	})

	t.Run("WriteStatus", func(t *testing.T) {
		t.Parallel()

		assert := require.New(t)

		_, rec, _, c := generateTestContext(http.MethodPost, "/api/path", nil)
		c.WriteStatus(http.StatusNoContent)
		assert.Equal(http.StatusNoContent, rec.Code)
	})

	t.Run("Redirect", func(t *testing.T) {
		t.Parallel()

		assert := require.New(t)

		_, rec, _, c := generateTestContext(http.MethodPost, "/api/path", nil)
		c.Redirect(http.StatusTemporaryRedirect, "https://example.com")
		assert.Equal(http.StatusTemporaryRedirect, rec.Code)
		assert.Equal("https://example.com", rec.Result().Header.Get("Location"))
	})

	t.Run("WriteString", func(t *testing.T) {
		t.Parallel()

		assert := require.New(t)

		_, rec, _, c := generateTestContext(http.MethodPost, "/api/path", nil)
		c.WriteString(http.StatusOK, "string response data")
		assert.Equal(http.StatusOK, rec.Code)
		assert.Equal("string response data", rec.Body.String())
	})

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
				LogError: "Error response\n[[\n(500) test error response message [err_code_890]\n]]\n--\n%!(STACKTRACE)\n--\ntest root error",
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
				LogError: "some message\n--\nError response\n[[\n(400) test error [test_err_code]\n]]\n--\n%!(STACKTRACE)\n--\ntest struct err",
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
			t.Run(tc.Test, func(t *testing.T) {
				t.Parallel()

				assert := require.New(t)

				var logbuf bytes.Buffer
				log := klog.New(
					klog.OptHandler(klog.NewJSONSlogHandler(klog.NewSyncWriter(&logbuf))),
				)
				req := httptest.NewRequest(http.MethodPost, tc.Path, strings.NewReader(tc.Body))
				req.Header.Set(headerContentType, mime.FormatMediaType("application/json", map[string]string{"charset": "utf-8"}))
				rec := httptest.NewRecorder()
				c := NewContext(rec, req, log)
				c.WriteError(tc.Err)
				assert.Equal(tc.Status, rec.Code)
				assert.Equal(tc.Res, strings.TrimSpace(rec.Body.String()))
				if tc.NoLog {
					assert.Equal(0, logbuf.Len())
					return
				}

				var j struct {
					Level string `json:"level"`
					Msg   string `json:"msg"`
					Err   struct {
						Msg   string `json:"msg"`
						Trace string `json:"trace"`
					} `json:"err"`
				}
				d := json.NewDecoder(&logbuf)
				assert.NoError(d.Decode(&j))
				assert.Equal(tc.Level, j.Level)
				assert.Equal(tc.LogMsg, j.Msg)
				if tc.NoTrace {
					assert.Equal(tc.LogError, j.Err.Msg)
					assert.Equal("NONE", j.Err.Trace)
				} else {
					assert.Regexp(stackRegex, j.Err.Msg)
					assert.Equal(tc.LogError, stackRegex.ReplaceAllString(j.Err.Msg, "%!(STACKTRACE)"))
				}
				assert.False(d.More())
			})
		}
	})

	t.Run("Websocket", func(t *testing.T) {
		t.Parallel()

		assert := require.New(t)

		var logbuf threadSafeBuffer
		log := klog.NewLevelLogger(klog.New(
			klog.OptHandler(klog.NewJSONSlogHandler(klog.NewSyncWriter(&logbuf))),
		))

		server := httptest.NewServer(toHTTPHandler(RouteHandlerFunc(func(c *Context) {
			conn, err := c.Websocket([]string{WSProtocolVersion})
			if err != nil {
				log.WarnErr(c.Ctx(), kerrors.WithMsg(err, "Failed to accept WS conn upgrade"))
				return
			}
			if conn.Subprotocol() != WSProtocolVersion {
				conn.CloseError(ErrWS(nil, int(websocket.StatusPolicyViolation), "Invalid ws subprotocol"))
				return
			}
			defer conn.Close(int(websocket.StatusInternalError), "Internal error")

			for {
				isText, b, err := conn.Read(c.Ctx())
				if err != nil {
					conn.CloseError(err)
					return
				}
				if !isText {
					conn.CloseError(ErrWS(nil, int(websocket.StatusUnsupportedData), "Invalid msg type binary"))
					return
				}
				var req struct {
					Channel string          `json:"channel"`
					Value   json.RawMessage `json:"value"`
				}
				if err := kjson.Unmarshal(b, &req); err != nil {
					conn.CloseError(ErrWS(err, int(websocket.StatusUnsupportedData), "Malformed request msg"))
					return
				}
				if req.Channel != "echo" {
					conn.CloseError(ErrWS(nil, int(websocket.StatusUnsupportedData), "Invalid msg channel"))
					return
				}
				if err := conn.Write(c.Ctx(), true, b); err != nil {
					conn.CloseError(err)
					return
				}
			}
		}), log.Logger))
		t.Cleanup(func() {
			server.Close()
		})

		wsurl, err := url.Parse(server.URL)
		assert.NoError(err)
		wsurl.Scheme = "ws"
		conn, _, err := websocket.Dial(context.Background(), wsurl.String(), &websocket.DialOptions{
			Subprotocols:    []string{WSProtocolVersion},
			CompressionMode: websocket.CompressionContextTakeover,
		})
		assert.NoError(err)
		t.Cleanup(func() {
			conn.Close(websocket.StatusInternalError, "abort")
		})

		assert.NoError(conn.Write(context.Background(), websocket.MessageText, []byte(`{"channel":"echo","value":{"ping":"pong"}}`)))
		msgType, resb, err := conn.Read(context.Background())
		assert.NoError(err)
		assert.Equal(websocket.MessageText, msgType)
		var res struct {
			Channel string `json:"channel"`
			Value   struct {
				Ping string `json:"ping"`
			} `json:"value"`
		}
		assert.NoError(kjson.Unmarshal(resb, &res))
		assert.Equal("echo", res.Channel)
		assert.Equal("pong", res.Value.Ping)
		conn.Close(websocket.StatusNormalClosure, "OK")
	})
}
