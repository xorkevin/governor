package governor

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	valuer interface {
		Value() interface{}
	}

	cloner interface {
		CloneEmptyPointer() valuer
	}

	testServiceA struct {
		log      *klog.LevelLogger
		name     string
		ranInit  bool
		ranStart bool
		ranSetup bool
		ranStop  bool
		healthy  *atomic.Bool
	}

	testServiceASecret struct {
		Secret string `mapstructure:"secret"`
	}

	testServiceAReq struct {
		Ping string `json:"ping"`
	}

	ctxKeyTestServiceA struct{}
)

func newTestServiceA() *testServiceA {
	return &testServiceA{
		healthy: &atomic.Bool{},
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

	if err := r.GetSecret(ctx, "bogus", 60, &secret); err == nil {
		return kerrors.WithMsg(nil, "Did not reject bogus secret")
	}

	mr := NewMethodRouter(m)
	mr.PostCtx("/ping", s.ping)

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
	c.WriteJSON(http.StatusOK, testServiceAReq{
		Ping: "pong",
	})
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
)

func TestServer(t *testing.T) {
	t.Parallel()

	tabReplacer := strings.NewReplacer("\t", "  ")

	t.Run("ServeHTTP", func(t *testing.T) {
		t.Parallel()

		for _, tc := range []struct {
			Test       string
			Method     string
			Path       string
			ReqHeaders map[string]string
			RemoteAddr string
			Body       io.Reader
			Status     int
			ResHeaders map[string]string
			ResBody    cloner
			Logs       []cloner
		}{
			{
				Test: "logs the error",
				ReqHeaders: map[string]string{
					headerContentType:   "application/json",
					headerXForwardedFor: "10.0.0.5, 10.0.0.4, 10.0.0.3",
				},
				RemoteAddr: "10.0.0.2:1234",
				Method:     http.MethodPost,
				Path:       "/api/servicea/ping",
				Body:       strings.NewReader(`{"ping": "ping"}`),
				Status:     http.StatusOK,
				ResHeaders: map[string]string{
					headerContentType: "application/json; charset=utf-8",
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
		} {
			tc := tc
			t.Run(tc.Test, func(t *testing.T) {
				t.Parallel()

				var logbuf bytes.Buffer

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
cors:
	alloworigins:
		- 'http://localhost:3000'
	allowpaths:
		- '^/api/servicea/ping$'
routerewrite:
	-
		host: localhost:8080
		methods: ['POST']
		pattern: '^/.well-known/([A-Za-z0-9_-]{2,})$'
		replace: '/api/servicea/$1'
trustedproxies:
	- '10.0.0.0/8'
setupsecret: setupsecret
servicea:
	prop2: yetanothervalue
	somesecret: s1secret
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

				serviceA := newTestServiceA()
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
				req.Host = "localhost:8080"
				for k, v := range tc.ReqHeaders {
					req.Header.Set(k, v)
				}
				req.RemoteAddr = tc.RemoteAddr
				rec := httptest.NewRecorder()
				server.ServeHTTP(rec, req)

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

				if tc.ResBody != nil {
					res := tc.ResBody.CloneEmptyPointer()
					assert.NoError(json.Unmarshal(rec.Body.Bytes(), res))
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
					Route:   tc.Path,
					Status:  tc.Status,
				}, logEntry)

				assert.False(j.More())
			})
		}
	})
}
