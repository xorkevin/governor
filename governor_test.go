package governor

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	wrappedReader struct {
		r io.Reader
	}
)

func (r wrappedReader) Read(p []byte) (int, error) {
	return r.r.Read(p)
}

type (
	valuer interface {
		Value() interface{}
	}

	cloner interface {
		CloneEmptyPointer() valuer
	}

	testServiceACheck struct {
		RealIP  string
		Status  int
		ResBody interface{}
	}

	testServiceA struct {
		log      *klog.LevelLogger
		name     string
		ranInit  bool
		ranStart bool
		ranSetup bool
		ranStop  bool
		healthy  *atomic.Bool
		check    testServiceACheck
	}

	testServiceASecret struct {
		Secret string `mapstructure:"secret"`
	}

	testServiceAReq struct {
		Ping string `json:"ping"`
	}

	ctxKeyTestServiceA struct{}
)

func newTestServiceA(check testServiceACheck) *testServiceA {
	return &testServiceA{
		healthy: &atomic.Bool{},
		check:   check,
	}
}

func (s *testServiceA) Register(inj Injector, r ConfigRegistrar) {
	inj.Set(ctxKeyTestServiceA{}, s)

	s.name = r.Name()
	r.SetDefault("prop1", "somevalue")
	r.SetDefault("prop2", "anothervalue")
}

func (s *testServiceA) Init(ctx context.Context, r ConfigReader, l klog.Logger, m Router) error {
	s.log = klog.NewLevelLogger(l)

	if r.Config().Hostname == "" {
		return kerrors.WithMsg(nil, "Invalid hostname")
	}
	if r.Config().Instance == "" {
		return kerrors.WithMsg(nil, "Invalid instance")
	}

	if r.Config() != (Config{
		Appname: "govtest",
		Version: Version{
			Num:  "test",
			Hash: "dev",
		},
		Hostname: r.Config().Hostname,
		Instance: r.Config().Instance,
		Addr:     ":8080",
		BaseURL:  "/api",
	}) {
		return kerrors.WithMsg(nil, "Invalid config")
	}

	if r.Name() != "servicea" {
		return kerrors.WithMsg(nil, "Invalid name")
	}
	if r.URL() != "/servicea" {
		return kerrors.WithMsg(nil, "Invalid url")
	}
	if r.GetStr("prop1") != "somevalue" {
		return kerrors.WithMsg(nil, "Invalid prop1")
	}
	if r.GetStr("prop2") != "yetanothervalue" {
		return kerrors.WithMsg(nil, "Invalid prop2")
	}

	var secret testServiceASecret
	if err := r.GetSecret(ctx, "somesecret", 60, &secret); err != nil {
		return kerrors.WithMsg(err, "Malformed secret")
	}
	if secret.Secret != "secretval" {
		return kerrors.WithMsg(nil, "Invalid secret")
	}
	// hits cache
	if err := r.GetSecret(ctx, "somesecret", 60, &secret); err != nil {
		return kerrors.WithMsg(err, "Malformed secret")
	}
	if secret.Secret != "secretval" {
		return kerrors.WithMsg(nil, "Invalid secret")
	}

	r.InvalidateSecret("somesecret")

	if err := r.GetSecret(ctx, "bogussecret", 60, &secret); err == nil {
		return kerrors.WithMsg(nil, "Did not reject bogus secret")
	}

	if !r.GetBool("propbool") {
		return kerrors.WithMsg(nil, "Invalid propbool")
	}
	if r.GetInt("propint") != 271828 {
		return kerrors.WithMsg(nil, "Invalid propint")
	}
	if r.GetInt("propint") != 271828 {
		return kerrors.WithMsg(nil, "Invalid propint")
	}
	dur, err := r.GetDuration("propdur")
	if err != nil {
		return kerrors.WithMsg(err, "Invalid propdur")
	}
	if dur != 24*time.Hour {
		return kerrors.WithMsg(nil, "Invalid propdur")
	}
	if list := r.GetStrSlice("propstrslice"); len(list) != 2 || list[0] != "abc" || list[1] != "def" {
		return kerrors.WithMsg(nil, "Invalid propstrslice")
	}

	var obj []struct {
		Field1 string `json:"field1"`
	}
	if err := r.Unmarshal("propobj", &obj); err != nil {
		return kerrors.WithMsg(err, "Invalid propobj")
	}
	if len(obj) != 1 || obj[0].Field1 != "abc" {
		return kerrors.WithMsg(err, "Invalid propobj")
	}

	m = m.GroupCtx("", func(next RouteHandler) RouteHandler {
		return RouteHandlerFunc(func(c *Context) {
			c.AddHeader("Test-Custom-Header", "test-header-val")
			c.AddHeader("Test-Custom-Header", "another-header-val")
			c.DelHeader("Test-Custom-Header")
			c.AddHeader("Test-Custom-Header", "test-header-val")
			next.ServeHTTPCtx(c)
		})
	})
	mr := NewMethodRouter(m)
	mr.PostCtx("/ping/{rparam}", s.ping)
	mr.PostCtx("/customroute", s.customroute)

	s.ranInit = true

	return nil
}

func (s *testServiceA) Start(ctx context.Context) error {
	s.healthy.Store(true)
	s.ranStart = true
	return nil
}

func (s *testServiceA) Stop(ctx context.Context) {
	s.ranStop = true
}
func (s *testServiceA) Setup(ctx context.Context, req ReqSetup) error {
	s.ranSetup = true
	return nil
}

func (s *testServiceA) Health(ctx context.Context) error {
	if !s.healthy.Load() {
		return kerrors.WithMsg(nil, "Service A unhealthy")
	}
	return nil
}

func (s *testServiceA) ping(c *Context) {
	if c.Log() == nil {
		c.WriteError(ErrWithRes(nil, http.StatusInternalServerError, "", "No context logger"))
		return
	}
	if c.LReqID() == "" {
		c.WriteError(ErrWithRes(nil, http.StatusInternalServerError, "", "No local request id"))
		return
	}
	username, password, ok := c.BasicAuth()
	if !ok || username != "admin" || password != "admin" {
		c.WriteError(ErrWithRes(nil, http.StatusUnauthorized, "", "Invalid user auth"))
		return
	}
	if c.RealIP() == nil || c.RealIP().String() != "192.168.0.3" {
		c.WriteError(ErrWithRes(nil, http.StatusForbidden, "", "Invalid calling ip"))
		return
	}
	if c.Param("rparam") != c.QueryDef("qparam", "unset") {
		c.WriteError(ErrWithRes(nil, http.StatusBadRequest, "", "Invalid path param"))
		return
	}
	if c.QueryDef("bogus", "a-value") != "a-value" {
		c.WriteError(ErrWithRes(nil, http.StatusBadRequest, "", "Bogus query param supplied"))
		return
	}
	if c.QueryInt("qparam", -1) != -1 {
		c.WriteError(ErrWithRes(nil, http.StatusBadRequest, "", "Invalid string param"))
		return
	}
	if c.QueryInt64("qparam", -1) != -1 {
		c.WriteError(ErrWithRes(nil, http.StatusBadRequest, "", "Invalid string param"))
		return
	}
	if c.QueryInt("bogus", -1) != -1 {
		c.WriteError(ErrWithRes(nil, http.StatusBadRequest, "", "Bogus query param supplied"))
		return
	}
	if c.QueryInt("iparam", -1) != 314159 {
		c.WriteError(ErrWithRes(nil, http.StatusBadRequest, "", "Invalid int param"))
		return
	}
	if c.QueryInt64("bogus", -1) != -1 {
		c.WriteError(ErrWithRes(nil, http.StatusBadRequest, "", "Bogus query param supplied"))
		return
	}
	if c.QueryInt64("iparam", -1) != 314159 {
		c.WriteError(ErrWithRes(nil, http.StatusBadRequest, "", "Invalid int64 param"))
		return
	}
	if c.QueryBool("bogus") {
		c.WriteError(ErrWithRes(nil, http.StatusBadRequest, "", "Bogus query param supplied"))
		return
	}
	if !c.QueryBool("bparam") {
		c.WriteError(ErrWithRes(nil, http.StatusBadRequest, "", "Invalid bool param"))
		return
	}
	cval, err := c.Cookie("user")
	if err != nil {
		c.WriteError(ErrWithRes(err, http.StatusUnauthorized, "", "Invalid user cookie"))
		return
	}
	if cval.Value == "" {
		c.WriteError(ErrWithRes(nil, http.StatusForbidden, "", "Invalid user cookie"))
		return
	}
	var req testServiceAReq
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	if req.Ping != "ping" {
		c.WriteError(ErrWithRes(nil, http.StatusBadRequest, "", "Must ping with ping"))
		return
	}
	s.log.Info(c.Ctx(), "Ping", klog.Fields{
		"ping": "pong",
	})
	c.SetCookie(&http.Cookie{
		Name:  "user",
		Value: cval.Value,
	})
	c.WriteJSON(http.StatusOK, testServiceAReq{
		Ping: "pong",
	})
}

func (s *testServiceA) customroute(c *Context) {
	if s.check.RealIP != "" {
		if c.RealIP() == nil || c.RealIP().String() != s.check.RealIP {
			c.WriteError(ErrWithRes(nil, http.StatusBadRequest, "", "Invalid real ip"))
			return
		}
	} else {
		if c.RealIP() != nil {
			c.WriteError(ErrWithRes(nil, http.StatusBadRequest, "", "Invalid real ip"))
			return
		}
	}
	if _, err := c.ReadAllBody(); err != nil {
		c.WriteError(err)
		return
	}

	if s.check.ResBody != nil {
		c.WriteJSON(s.check.Status, s.check.ResBody)
	} else {
		c.WriteStatus(s.check.Status)
	}
}

func (r testServiceAReq) CloneEmptyPointer() valuer {
	return &testServiceAReq{}
}

func (r *testServiceAReq) Value() interface{} {
	return *r
}

type (
	testLogEntryMsg struct {
		Msg     string `json:"msg"`
		Status  int    `json:"http.status"`
		ReqPath string `json:"http.reqpath"`
		Route   string `json:"http.route"`
	}

	testLogErrorMsg struct {
		Msg     string `json:"msg"`
		ReqPath string `json:"http.reqpath"`
	}
)

func (r testLogErrorMsg) CloneEmptyPointer() valuer {
	return &testLogErrorMsg{}
}

func (r *testLogErrorMsg) Value() interface{} {
	return *r
}

func TestServer(t *testing.T) {
	t.Parallel()

	tabReplacer := strings.NewReplacer("\t", "  ")

	t.Run("ServeHTTP", func(t *testing.T) {
		t.Parallel()

		for _, tc := range []struct {
			Test       string
			Method     string
			Path       string
			Query      url.Values
			Route      string
			ReqHeaders map[string]string
			RemoteAddr string
			Username   string
			Password   string
			UserCookie string
			Body       io.Reader
			Status     int
			ResHeaders map[string]string
			ResBody    cloner
			Compressed string
			Logs       []cloner
			MaxReqSize string
			Check      testServiceACheck
		}{
			{
				Test:   "handles a request",
				Method: http.MethodPost,
				Path:   "/api/servicea/ping/paramvalue/",
				Query: url.Values{
					"qparam": []string{"paramvalue"},
					"iparam": []string{"314159"},
					"bparam": []string{"t"},
				},
				Route: "/api/servicea/ping/{rparam}",
				ReqHeaders: map[string]string{
					headerContentType:   "application/json",
					headerXForwardedFor: "192.168.0.3, 10.0.0.5, 10.0.0.4, 10.0.0.3",
				},
				RemoteAddr: "10.0.0.2:1234",
				Username:   "admin",
				Password:   "admin",
				UserCookie: "admin",
				Body:       strings.NewReader(`{"ping": "ping"}`),
				Status:     http.StatusOK,
				ResHeaders: map[string]string{
					"Test-Custom-Header": "test-header-val",
					headerContentType:    "application/json; charset=utf-8",
				},
				ResBody: testServiceAReq{
					Ping: "pong",
				},
				Logs: []cloner{
					testServiceAReq{
						Ping: "pong",
					},
				},
			},
			{
				Test:   "handles route rewrite",
				Method: http.MethodPost,
				Path:   "/.well-known/ping/paramvalue",
				Query: url.Values{
					"qparam": []string{"paramvalue"},
					"iparam": []string{"314159"},
					"bparam": []string{"t"},
				},
				Route: "/api/servicea/ping/{rparam}",
				ReqHeaders: map[string]string{
					headerContentType:   "application/json",
					headerXForwardedFor: "192.168.0.3, 10.0.0.5, 10.0.0.4, 10.0.0.3",
				},
				RemoteAddr: "10.0.0.2:1234",
				Username:   "admin",
				Password:   "admin",
				UserCookie: "admin",
				Body:       strings.NewReader(`{"ping": "ping"}`),
				Status:     http.StatusOK,
				ResHeaders: map[string]string{
					"Test-Custom-Header": "test-header-val",
					headerContentType:    "application/json; charset=utf-8",
				},
				ResBody: testServiceAReq{
					Ping: "pong",
				},
				Logs: []cloner{
					testServiceAReq{
						Ping: "pong",
					},
				},
			},
			{
				Test:   "handles cors preflight",
				Method: http.MethodOptions,
				Path:   "/api/servicea/ping/paramvalue",
				Route:  "",
				ReqHeaders: map[string]string{
					headerXForwardedFor:              "10.0.0.5, 10.0.0.4, 10.0.0.3",
					"Origin":                         "http://localhost:3000",
					"Access-Control-Request-Method":  http.MethodPost,
					"Access-Control-Request-Headers": "origin",
				},
				RemoteAddr: "10.0.0.2:1234",
				Status:     http.StatusOK,
			},
			{
				Test:   "handles cors preflight",
				Method: http.MethodOptions,
				Path:   "/api/servicea/ping/allowall",
				Route:  "",
				ReqHeaders: map[string]string{
					headerXForwardedFor:              "192.168.0.3, bogus, 10.0.0.5, 10.0.0.4, 10.0.0.3",
					"Origin":                         "http://localhost:3000",
					"Access-Control-Request-Method":  http.MethodPost,
					"Access-Control-Request-Headers": "origin",
				},
				RemoteAddr: "10.0.0.2:1234",
				Status:     http.StatusOK,
			},
			{
				Test:   "realip checks remote addr for trust",
				Method: http.MethodPost,
				Path:   "/api/servicea/customroute",
				Route:  "/api/servicea/customroute",
				ReqHeaders: map[string]string{
					headerXForwardedFor: "192.168.0.3, 10.0.0.5, 10.0.0.4, 10.0.0.3",
				},
				RemoteAddr: "192.168.0.5:1234",
				Status:     http.StatusOK,
				Check: testServiceACheck{
					RealIP: "192.168.0.5",
					Status: http.StatusOK,
				},
			},
			{
				Test:       "realip falls back to remote addr",
				Method:     http.MethodPost,
				Path:       "/api/servicea/customroute",
				Route:      "/api/servicea/customroute",
				RemoteAddr: "10.0.0.5:1234",
				Status:     http.StatusOK,
				Check: testServiceACheck{
					RealIP: "10.0.0.5",
					Status: http.StatusOK,
				},
			},
			{
				Test:       "realip handles failure to parse remote addr",
				Method:     http.MethodPost,
				Path:       "/api/servicea/customroute",
				Route:      "/api/servicea/customroute",
				RemoteAddr: "bogus",
				Status:     http.StatusOK,
				Check: testServiceACheck{
					RealIP: "",
					Status: http.StatusOK,
				},
			},
			{
				Test:       "max bytes reader limits request size for content length",
				Method:     http.MethodPost,
				Path:       "/api/servicea/customroute",
				Route:      "",
				RemoteAddr: "192.168.0.3:1234",
				Body:       strings.NewReader("This is a string that is longer than 16 bytes"),
				Status:     http.StatusRequestEntityTooLarge,
				Logs: []cloner{
					testLogErrorMsg{
						Msg:     "Error response",
						ReqPath: "/api/servicea/customroute",
					},
				},
				Check: testServiceACheck{
					RealIP: "192.168.0.3",
					Status: http.StatusOK,
				},
				MaxReqSize: "16B",
			},
			{
				Test:       "max bytes reader limits request size",
				Method:     http.MethodPost,
				Path:       "/api/servicea/customroute",
				Route:      "/api/servicea/customroute",
				RemoteAddr: "192.168.0.3:1234",
				Body:       wrappedReader{strings.NewReader("This is a string that is longer than 16 bytes")},
				Status:     http.StatusRequestEntityTooLarge,
				Logs: []cloner{
					testLogErrorMsg{
						Msg:     "Error response",
						ReqPath: "/api/servicea/customroute",
					},
				},
				Check: testServiceACheck{
					RealIP: "192.168.0.3",
					Status: http.StatusOK,
				},
				MaxReqSize: "16B",
			},
			{
				Test:   "compressor compresses gzip",
				Method: http.MethodPost,
				Path:   "/api/servicea/customroute",
				Route:  "/api/servicea/customroute",
				ReqHeaders: map[string]string{
					headerAcceptEncoding: encodingKindGzip + ", " + encodingKindZlib,
				},
				RemoteAddr: "192.168.0.3:1234",
				Status:     http.StatusOK,
				ResHeaders: map[string]string{
					headerContentType: "application/json; charset=utf-8",
				},
				ResBody: testServiceAReq{
					Ping: "this is the ping field that should be compressed",
				},
				Compressed: encodingKindGzip,
				Check: testServiceACheck{
					RealIP: "192.168.0.3",
					Status: http.StatusOK,
					ResBody: testServiceAReq{
						Ping: "this is the ping field that should be compressed",
					},
				},
			},
			{
				Test:   "compressor compresses zlib",
				Method: http.MethodPost,
				Path:   "/api/servicea/customroute",
				Route:  "/api/servicea/customroute",
				ReqHeaders: map[string]string{
					headerAcceptEncoding: encodingKindZlib,
				},
				RemoteAddr: "192.168.0.3:1234",
				Status:     http.StatusOK,
				ResHeaders: map[string]string{
					headerContentType: "application/json; charset=utf-8",
				},
				ResBody: testServiceAReq{
					Ping: "this is the ping field that should be compressed",
				},
				Compressed: encodingKindZlib,
				Check: testServiceACheck{
					RealIP: "192.168.0.3",
					Status: http.StatusOK,
					ResBody: testServiceAReq{
						Ping: "this is the ping field that should be compressed",
					},
				},
			},
			{
				Test:   "compressor compresses zstd",
				Method: http.MethodPost,
				Path:   "/api/servicea/customroute",
				Route:  "/api/servicea/customroute",
				ReqHeaders: map[string]string{
					headerAcceptEncoding: encodingKindZstd + ", " + encodingKindGzip + ", " + encodingKindZlib,
				},
				RemoteAddr: "192.168.0.3:1234",
				Status:     http.StatusOK,
				ResHeaders: map[string]string{
					headerContentType: "application/json; charset=utf-8",
				},
				ResBody: testServiceAReq{
					Ping: "this is the ping field that should be compressed",
				},
				Compressed: encodingKindZstd,
				Check: testServiceACheck{
					RealIP: "192.168.0.3",
					Status: http.StatusOK,
					ResBody: testServiceAReq{
						Ping: "this is the ping field that should be compressed",
					},
				},
			},
		} {
			tc := tc
			t.Run(tc.Test, func(t *testing.T) {
				t.Parallel()

				var logbuf bytes.Buffer

				maxreqsize := "2MB"
				if tc.MaxReqSize != "" {
					maxreqsize = tc.MaxReqSize
				}

				server := New(Opts{
					Appname: "govtest",
					Version: Version{
						Num:  "test",
						Hash: "dev",
					},
					Description:  "test gov server",
					EnvPrefix:    "gov",
					ClientPrefix: "govc",
					ConfigReader: strings.NewReader(tabReplacer.Replace(`
http:
	addr: ':8080'
	baseurl: /api
	maxreqsize: ` + maxreqsize + `
cors:
	alloworigins:
		- 'http://localhost:3000'
	allowpaths:
		- '^/api/servicea/ping/allowall$'
routerewrite:
	-
		host: localhost:8080
		methods: ['POST']
		pattern: '^/.well-known/(.+)$'
		replace: '/api/servicea/$1'
trustedproxies:
	- '10.0.0.0/8'
setupsecret: setupsecret
servicea:
	prop2: yetanothervalue
	somesecret: s1secret
	bogussecret: bogussecret
	propbool: true
	propint: 271828
	propdur: 24h
	propstrslice:
		- abc
		- def
	propobj:
		-
			field1: abc
`)),
					VaultReader: strings.NewReader(tabReplacer.Replace(`
data:
	setupsecret:
		secret: setupsecret
	s1secret:
		secret: secretval
`)),
					LogWriter: &logbuf,
				})

				assert := require.New(t)

				serviceA := newTestServiceA(tc.Check)
				server.Register("servicea", "/servicea", serviceA)

				assert.Equal("servicea", serviceA.name)

				server.SetFlags(Flags{})

				assert.NoError(server.Init(context.Background()))
				// does not reinit if already init-ed
				assert.NoError(server.Init(context.Background()))

				assert.True(serviceA.ranInit)
				assert.True(serviceA.ranStart)
				assert.False(serviceA.ranSetup)
				assert.False(serviceA.ranStop)

				t.Cleanup(func() {
					server.Stop(context.Background())
					// does not stop again if already stopped
					server.Stop(context.Background())

					assert.True(serviceA.ranStop)
				})

				logbuf.Reset()

				req := httptest.NewRequest(tc.Method, tc.Path, tc.Body)
				if tc.Query != nil {
					req.URL.RawQuery = tc.Query.Encode()
				}
				req.Host = "localhost:8080"
				for k, v := range tc.ReqHeaders {
					req.Header.Set(k, v)
				}
				req.RemoteAddr = tc.RemoteAddr
				if tc.Username != "" {
					req.SetBasicAuth(tc.Username, tc.Password)
				}
				if tc.UserCookie != "" {
					req.AddCookie(&http.Cookie{
						Name:  "user",
						Value: tc.UserCookie,
					})
				}
				rec := httptest.NewRecorder()
				server.ServeHTTP(rec, req)

				tc.Path = strings.TrimRight(tc.Path, "/")

				assert.Equal(tc.Status, rec.Code)

				j := json.NewDecoder(&logbuf)

				var logEntry testLogEntryMsg
				assert.NoError(j.Decode(&logEntry))
				assert.Equal(testLogEntryMsg{
					Msg:     "HTTP request",
					ReqPath: tc.Path,
				}, logEntry)

				for k, v := range tc.ResHeaders {
					assert.Equal(v, rec.Result().Header.Get(k))
				}

				var resUserCookieVal string
				for _, i := range rec.Result().Cookies() {
					if i.Name == "user" {
						resUserCookieVal = i.Value
					}
				}
				assert.Equal(tc.UserCookie, resUserCookieVal)

				if tc.ResBody != nil {
					resbody := rec.Body.Bytes()

					if tc.Compressed != "" {
						assert.Equal(tc.Compressed, rec.Result().Header.Get(headerContentEncoding))

						switch tc.Compressed {
						case encodingKindZstd:
							{
								r, err := zstd.NewReader(bytes.NewReader(resbody))
								assert.NoError(err)
								var b bytes.Buffer
								_, err = io.Copy(&b, r)
								assert.NoError(err)
								resbody = b.Bytes()
							}
						case encodingKindGzip:
							{
								r, err := gzip.NewReader(bytes.NewReader(resbody))
								assert.NoError(err)
								var b bytes.Buffer
								_, err = io.Copy(&b, r)
								assert.NoError(err)
								resbody = b.Bytes()
							}
						case encodingKindZlib:
							{
								r, err := zlib.NewReader(bytes.NewReader(resbody))
								assert.NoError(err)
								var b bytes.Buffer
								_, err = io.Copy(&b, r)
								assert.NoError(err)
								resbody = b.Bytes()
							}
						default:
							assert.FailNow("test encoding kind unsupported")
						}
					}

					res := tc.ResBody.CloneEmptyPointer()
					assert.NoError(json.Unmarshal(resbody, res))
					assert.Equal(tc.ResBody, res.Value())
				}

				for _, i := range tc.Logs {
					logEntry := i.CloneEmptyPointer()
					assert.NoError(j.Decode(logEntry))
					assert.Equal(i, logEntry.Value())
				}

				assert.NoError(j.Decode(&logEntry))
				assert.Equal(testLogEntryMsg{
					Msg:     "HTTP response",
					ReqPath: tc.Path,
					Route:   tc.Route,
					Status:  tc.Status,
				}, logEntry)

				assert.False(j.More())
			})
		}
	})
}
