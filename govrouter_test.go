package governor

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"xorkevin.dev/klog"
)

type (
	testServiceB struct {
		log *klog.LevelLogger
	}

	testServiceBReq struct {
		Method string `json:"method"`
		Path   string `json:"path"`
		Err    string `json:"err,omitempty"`
	}
)

func (s *testServiceB) Register(inj Injector, r ConfigRegistrar) {
}

func (s *testServiceB) Init(ctx context.Context, r ConfigReader, l klog.Logger, m Router) error {
	s.log = klog.NewLevelLogger(l)

	m = m.Group("", func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Common", "common")
			next.ServeHTTP(w, r)
		})
	})
	first := m.Group("/first", func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("First", "first")
			next.ServeHTTP(w, r)
		})
	})
	m1 := NewMethodRouter(first)
	m1.GetCtx("", s.echo)
	m1.PostCtx("", s.echo)
	m1.PutCtx("", s.echo)
	m1.PatchCtx("", s.echo)
	m1.DeleteCtx("", s.echo)
	m1.AnyCtx("/any", s.echo)
	m1.GetCtx("/specific", s.echo)
	second := m.GroupCtx("/second", func(next RouteHandler) RouteHandler {
		return RouteHandlerFunc(func(c *Context) {
			c.SetHeader("Second", "second")
			next.ServeHTTPCtx(c)
		})
	})
	m2 := NewMethodRouter(second)
	m2.Get("", s.httpecho)
	m2.Post("", s.httpecho)
	m2.Put("", s.httpecho)
	m2.Patch("", s.httpecho)
	m2.Delete("", s.httpecho)
	m2.Any("/any", s.httpecho, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Second", "secondany")
			next.ServeHTTP(w, r)
		})
	})
	m2.Get("/specific", s.httpecho)
	second.NotFound(http.HandlerFunc(s.httpnotfound), func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Not-Found", "secondnotfound")
			next.ServeHTTP(w, r)
		})
	})
	second.MethodNotAllowed(http.HandlerFunc(s.httpmethodnotallowed), func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Method-Not-Allowed", "secondmethodnotallowed")
			next.ServeHTTP(w, r)
		})
	})
	m.NotFoundCtx(RouteHandlerFunc(s.notfound), func(next RouteHandler) RouteHandler {
		return RouteHandlerFunc(func(c *Context) {
			c.SetHeader("Not-Found", "notfound")
			next.ServeHTTPCtx(c)
		})
	})
	m.MethodNotAllowedCtx(RouteHandlerFunc(s.methodnotallowed), func(next RouteHandler) RouteHandler {
		return RouteHandlerFunc(func(c *Context) {
			c.SetHeader("Method-Not-Allowed", "methodnotallowed")
			next.ServeHTTPCtx(c)
		})
	})
	return nil
}

func (s *testServiceB) Start(ctx context.Context) error {
	return nil
}

func (s *testServiceB) Stop(ctx context.Context) {
}

func (s *testServiceB) Setup(ctx context.Context, req ReqSetup) error {
	return nil
}

func (s *testServiceB) Health(ctx context.Context) error {
	return nil
}

func (s *testServiceB) echo(c *Context) {
	c.WriteJSON(http.StatusOK, testServiceBReq{
		Method: c.Req().Method,
		Path:   c.Req().URL.EscapedPath(),
	})
}

func (s *testServiceB) httpecho(w http.ResponseWriter, r *http.Request) {
	s.echo(NewContext(w, r, s.log.Logger))
}

func (s *testServiceB) notfound(c *Context) {
	c.WriteJSON(http.StatusNotFound, testServiceBReq{
		Method: c.Req().Method,
		Path:   c.Req().URL.EscapedPath(),
		Err:    "not found",
	})
}

func (s *testServiceB) httpnotfound(w http.ResponseWriter, r *http.Request) {
	s.notfound(NewContext(w, r, s.log.Logger))
}

func (s *testServiceB) methodnotallowed(c *Context) {
	c.WriteJSON(http.StatusMethodNotAllowed, testServiceBReq{
		Method: c.Req().Method,
		Path:   c.Req().URL.EscapedPath(),
		Err:    "method not allowed",
	})
}

func (s *testServiceB) httpmethodnotallowed(w http.ResponseWriter, r *http.Request) {
	s.methodnotallowed(NewContext(w, r, s.log.Logger))
}

func (r testServiceBReq) CloneEmptyPointer() valuer {
	return &testServiceBReq{}
}

func (r *testServiceBReq) Value() interface{} {
	return *r
}

func TestRouter(t *testing.T) {
	t.Parallel()

	t.Run("ServeHTTP", func(t *testing.T) {
		t.Parallel()

		for _, tc := range []struct {
			Test       string
			Method     string
			Path       string
			Body       io.Reader
			Status     int
			ResHeaders map[string]string
			ResBody    cloner
		}{
			{
				Test:   "handles a ctx get request",
				Method: http.MethodGet,
				Path:   "/api/serviceb/first",
				Status: http.StatusOK,
				ResHeaders: map[string]string{
					"Common":             "common",
					"First":              "first",
					"Second":             "",
					"Not-Found":          "",
					"Method-Not-Allowed": "",
				},
				ResBody: testServiceBReq{
					Method: http.MethodGet,
					Path:   "/api/serviceb/first",
				},
			},
			{
				Test:   "handles a ctx post request",
				Method: http.MethodPost,
				Path:   "/api/serviceb/first",
				Status: http.StatusOK,
				ResHeaders: map[string]string{
					"Common":             "common",
					"First":              "first",
					"Second":             "",
					"Not-Found":          "",
					"Method-Not-Allowed": "",
				},
				ResBody: testServiceBReq{
					Method: http.MethodPost,
					Path:   "/api/serviceb/first",
				},
			},
			{
				Test:   "handles a ctx put request",
				Method: http.MethodPut,
				Path:   "/api/serviceb/first",
				Status: http.StatusOK,
				ResHeaders: map[string]string{
					"Common":             "common",
					"First":              "first",
					"Second":             "",
					"Not-Found":          "",
					"Method-Not-Allowed": "",
				},
				ResBody: testServiceBReq{
					Method: http.MethodPut,
					Path:   "/api/serviceb/first",
				},
			},
			{
				Test:   "handles a ctx patch request",
				Method: http.MethodPatch,
				Path:   "/api/serviceb/first",
				Status: http.StatusOK,
				ResHeaders: map[string]string{
					"Common":             "common",
					"First":              "first",
					"Second":             "",
					"Not-Found":          "",
					"Method-Not-Allowed": "",
				},
				ResBody: testServiceBReq{
					Method: http.MethodPatch,
					Path:   "/api/serviceb/first",
				},
			},
			{
				Test:   "handles a ctx delete request",
				Method: http.MethodDelete,
				Path:   "/api/serviceb/first",
				Status: http.StatusOK,
				ResHeaders: map[string]string{
					"Common":             "common",
					"First":              "first",
					"Second":             "",
					"Not-Found":          "",
					"Method-Not-Allowed": "",
				},
				ResBody: testServiceBReq{
					Method: http.MethodDelete,
					Path:   "/api/serviceb/first",
				},
			},
			{
				Test:   "handles a ctx any put request",
				Method: http.MethodPut,
				Path:   "/api/serviceb/first/any",
				Status: http.StatusOK,
				ResHeaders: map[string]string{
					"Common":             "common",
					"First":              "first",
					"Second":             "",
					"Not-Found":          "",
					"Method-Not-Allowed": "",
				},
				ResBody: testServiceBReq{
					Method: http.MethodPut,
					Path:   "/api/serviceb/first/any",
				},
			},
			{
				Test:   "handles a ctx any post request",
				Method: http.MethodPost,
				Path:   "/api/serviceb/first/any",
				Status: http.StatusOK,
				ResHeaders: map[string]string{
					"Common":             "common",
					"First":              "first",
					"Second":             "",
					"Not-Found":          "",
					"Method-Not-Allowed": "",
				},
				ResBody: testServiceBReq{
					Method: http.MethodPost,
					Path:   "/api/serviceb/first/any",
				},
			},
			{
				Test:   "handles a get request",
				Method: http.MethodGet,
				Path:   "/api/serviceb/second",
				Status: http.StatusOK,
				ResHeaders: map[string]string{
					"Common":             "common",
					"First":              "",
					"Second":             "second",
					"Not-Found":          "",
					"Method-Not-Allowed": "",
				},
				ResBody: testServiceBReq{
					Method: http.MethodGet,
					Path:   "/api/serviceb/second",
				},
			},
			{
				Test:   "handles a post request",
				Method: http.MethodPost,
				Path:   "/api/serviceb/second",
				Status: http.StatusOK,
				ResHeaders: map[string]string{
					"Common":             "common",
					"First":              "",
					"Second":             "second",
					"Not-Found":          "",
					"Method-Not-Allowed": "",
				},
				ResBody: testServiceBReq{
					Method: http.MethodPost,
					Path:   "/api/serviceb/second",
				},
			},
			{
				Test:   "handles a put request",
				Method: http.MethodPut,
				Path:   "/api/serviceb/second",
				Status: http.StatusOK,
				ResHeaders: map[string]string{
					"Common":             "common",
					"First":              "",
					"Second":             "second",
					"Not-Found":          "",
					"Method-Not-Allowed": "",
				},
				ResBody: testServiceBReq{
					Method: http.MethodPut,
					Path:   "/api/serviceb/second",
				},
			},
			{
				Test:   "handles a patch request",
				Method: http.MethodPatch,
				Path:   "/api/serviceb/second",
				Status: http.StatusOK,
				ResHeaders: map[string]string{
					"Common":             "common",
					"First":              "",
					"Second":             "second",
					"Not-Found":          "",
					"Method-Not-Allowed": "",
				},
				ResBody: testServiceBReq{
					Method: http.MethodPatch,
					Path:   "/api/serviceb/second",
				},
			},
			{
				Test:   "handles a delete request",
				Method: http.MethodDelete,
				Path:   "/api/serviceb/second",
				Status: http.StatusOK,
				ResHeaders: map[string]string{
					"Common":             "common",
					"First":              "",
					"Second":             "second",
					"Not-Found":          "",
					"Method-Not-Allowed": "",
				},
				ResBody: testServiceBReq{
					Method: http.MethodDelete,
					Path:   "/api/serviceb/second",
				},
			},
			{
				Test:   "handles an any put request",
				Method: http.MethodPut,
				Path:   "/api/serviceb/second/any",
				Status: http.StatusOK,
				ResHeaders: map[string]string{
					"Common":             "common",
					"First":              "",
					"Second":             "secondany",
					"Not-Found":          "",
					"Method-Not-Allowed": "",
				},
				ResBody: testServiceBReq{
					Method: http.MethodPut,
					Path:   "/api/serviceb/second/any",
				},
			},
			{
				Test:   "handles an any post request",
				Method: http.MethodPost,
				Path:   "/api/serviceb/second/any",
				Status: http.StatusOK,
				ResHeaders: map[string]string{
					"Common":             "common",
					"First":              "",
					"Second":             "secondany",
					"Not-Found":          "",
					"Method-Not-Allowed": "",
				},
				ResBody: testServiceBReq{
					Method: http.MethodPost,
					Path:   "/api/serviceb/second/any",
				},
			},
			{
				Test:   "handles a ctx not found request",
				Method: http.MethodPost,
				Path:   "/api/serviceb/first/bogus",
				Status: http.StatusNotFound,
				ResHeaders: map[string]string{
					"Common":             "common",
					"First":              "",
					"Second":             "",
					"Not-Found":          "notfound",
					"Method-Not-Allowed": "",
				},
				ResBody: testServiceBReq{
					Method: http.MethodPost,
					Path:   "/api/serviceb/first/bogus",
					Err:    "not found",
				},
			},
			{
				Test:   "handles a ctx method not allowed request",
				Method: http.MethodPost,
				Path:   "/api/serviceb/first/specific",
				Status: http.StatusMethodNotAllowed,
				ResHeaders: map[string]string{
					"Common":             "common",
					"First":              "",
					"Second":             "",
					"Not-Found":          "",
					"Method-Not-Allowed": "methodnotallowed",
				},
				ResBody: testServiceBReq{
					Method: http.MethodPost,
					Path:   "/api/serviceb/first/specific",
					Err:    "method not allowed",
				},
			},
			{
				Test:   "handles a not found request",
				Method: http.MethodPost,
				Path:   "/api/serviceb/second/bogus",
				Status: http.StatusNotFound,
				ResHeaders: map[string]string{
					"Common":             "common",
					"First":              "",
					"Second":             "",
					"Not-Found":          "notfound",
					"Method-Not-Allowed": "",
				},
				ResBody: testServiceBReq{
					Method: http.MethodPost,
					Path:   "/api/serviceb/second/bogus",
					Err:    "not found",
				},
			},
			{
				Test:   "handles a method not allowed request",
				Method: http.MethodPost,
				Path:   "/api/serviceb/second/specific",
				Status: http.StatusMethodNotAllowed,
				ResHeaders: map[string]string{
					"Common":             "common",
					"First":              "",
					"Second":             "",
					"Not-Found":          "",
					"Method-Not-Allowed": "methodnotallowed",
				},
				ResBody: testServiceBReq{
					Method: http.MethodPost,
					Path:   "/api/serviceb/second/specific",
					Err:    "method not allowed",
				},
			},
		} {
			tc := tc
			t.Run(tc.Test, func(t *testing.T) {
				t.Parallel()

				server := New(Opts{
					Appname: "govtest",
					Version: Version{
						Num:  "test",
						Hash: "dev",
					},
					Description:  "test gov server",
					EnvPrefix:    "gov",
					ClientPrefix: "govc",
				}, &ServerOpts{
					ConfigReader: strings.NewReader(`
{
  "http": {
    "addr": ":8080",
    "basepath": "/api"
  },
  "setupsecret": "setupsecret"
}
`),
					VaultReader: strings.NewReader(`
{
  "data": {
    "setupsecret": {
      "secret": "setupsecret"
    }
  }
}
`),
				})

				assert := require.New(t)

				serviceB := &testServiceB{}
				server.Register("serviceb", "/serviceb", serviceB)

				assert.NoError(server.Init(context.Background(), Flags{}, klog.Discard{}))

				t.Cleanup(func() {
					server.Stop(context.Background())
				})

				req := httptest.NewRequest(tc.Method, tc.Path, tc.Body)
				rec := httptest.NewRecorder()
				server.ServeHTTP(rec, req)

				assert.Equal(tc.Status, rec.Code)

				for k, v := range tc.ResHeaders {
					assert.Equal(v, rec.Result().Header.Get(k))
				}

				if tc.ResBody != nil {
					resbody := rec.Body.Bytes()

					res := tc.ResBody.CloneEmptyPointer()
					assert.NoError(json.Unmarshal(resbody, res))
					assert.Equal(tc.ResBody, res.Value())
				}
			})
		}
	})
}
