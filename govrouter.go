package governor

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"nhooyr.io/websocket"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

const (
	WSProtocolVersion  = "xorkevin.dev-governor_ws_v1alpha1"
	WSReadLimitDefault = 32768
)

type (
	// Router adds route handlers to the server
	Router interface {
		Group(path string, mw ...Middleware) Router
		Method(method string, path string, fn http.HandlerFunc, mw ...Middleware)
		Get(path string, fn http.HandlerFunc, mw ...Middleware)
		Post(path string, fn http.HandlerFunc, mw ...Middleware)
		Put(path string, fn http.HandlerFunc, mw ...Middleware)
		Patch(path string, fn http.HandlerFunc, mw ...Middleware)
		Delete(path string, fn http.HandlerFunc, mw ...Middleware)
		Any(path string, fn http.HandlerFunc, mw ...Middleware)
		NotFound(fn http.HandlerFunc, mw ...Middleware)
		MethodNotAllowed(fn http.HandlerFunc, mw ...Middleware)
		GroupCtx(path string, mw ...MiddlewareCtx) Router
		MethodCtx(method string, path string, fn HandlerFunc, mw ...MiddlewareCtx)
		GetCtx(path string, fn HandlerFunc, mw ...MiddlewareCtx)
		PostCtx(path string, fn HandlerFunc, mw ...MiddlewareCtx)
		PutCtx(path string, fn HandlerFunc, mw ...MiddlewareCtx)
		PatchCtx(path string, fn HandlerFunc, mw ...MiddlewareCtx)
		DeleteCtx(path string, fn HandlerFunc, mw ...MiddlewareCtx)
		AnyCtx(path string, fn HandlerFunc, mw ...MiddlewareCtx)
		NotFoundCtx(fn HandlerFunc, mw ...MiddlewareCtx)
		MethodNotAllowedCtx(fn HandlerFunc, mw ...MiddlewareCtx)
	}

	govrouter struct {
		r    chi.Router
		log  klog.Logger
		path string
	}

	// HandlerFunc is a type alias for a router handler with Context
	HandlerFunc = func(c Context)
	// Middleware is a type alias for Router middleware
	Middleware = func(next http.Handler) http.Handler
	// MiddlewareCtx is a type alias for Router middleware with Context
	MiddlewareCtx = func(next HandlerFunc) HandlerFunc
)

func (s *Server) router(path string, l klog.Logger) Router {
	return &govrouter{
		r: s.i.Route(path, func(r chi.Router) {}),
		log: l.Sublogger("router", klog.Fields{
			"gov.router.path": path,
		}),
		path: path,
	}
}

func (r *govrouter) Group(path string, mw ...Middleware) Router {
	cpath := r.path + path
	sr := r.r.Route(path, func(r chi.Router) {})
	if len(mw) > 0 {
		sr = sr.With(mw...)
	}
	return &govrouter{
		r: sr,
		log: r.log.Sublogger("", klog.Fields{
			"gov.router.path": cpath,
		}),
		path: cpath,
	}
}

func (r *govrouter) method(method string, path string, fn http.HandlerFunc, mw []Middleware) {
	if path == "" {
		path = "/"
	}
	k := r.r
	if len(mw) > 0 {
		k = r.r.With(mw...)
	}
	k.MethodFunc(method, path, fn)
}

func (r *govrouter) Method(method string, path string, fn http.HandlerFunc, mw ...Middleware) {
	r.method(method, path, fn, mw)
}

func (r *govrouter) Get(path string, fn http.HandlerFunc, mw ...Middleware) {
	r.method(http.MethodGet, path, fn, mw)
}

func (r *govrouter) Post(path string, fn http.HandlerFunc, mw ...Middleware) {
	r.method(http.MethodPost, path, fn, mw)
}

func (r *govrouter) Put(path string, fn http.HandlerFunc, mw ...Middleware) {
	r.method(http.MethodPut, path, fn, mw)
}

func (r *govrouter) Patch(path string, fn http.HandlerFunc, mw ...Middleware) {
	r.method(http.MethodPatch, path, fn, mw)
}

func (r *govrouter) Delete(path string, fn http.HandlerFunc, mw ...Middleware) {
	r.method(http.MethodDelete, path, fn, mw)
}

func (r *govrouter) Any(path string, fn http.HandlerFunc, mw ...Middleware) {
	if path == "" {
		path = "/"
	}
	k := r.r
	if len(mw) > 0 {
		k = r.r.With(mw...)
	}
	k.HandleFunc(path, fn)
}

func (r *govrouter) NotFound(fn http.HandlerFunc, mw ...Middleware) {
	k := r.r
	if len(mw) > 0 {
		k = r.r.With(mw...)
	}
	k.NotFound(fn)
}

func (r *govrouter) MethodNotAllowed(fn http.HandlerFunc, mw ...Middleware) {
	k := r.r
	if len(mw) > 0 {
		k = r.r.With(mw...)
	}
	k.MethodNotAllowed(fn)
}

func (r *govrouter) chainMiddlewareCtx(fn HandlerFunc, mw []MiddlewareCtx) HandlerFunc {
	for i := len(mw) - 1; i >= 0; i-- {
		fn = mw[i](fn)
	}
	return fn
}

func (r *govrouter) toHTTPMiddleware(mw []MiddlewareCtx, log klog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		fn := r.chainMiddlewareCtx(func(c Context) {
			next.ServeHTTP(c.Res(), c.Req())
		}, mw)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fn(NewContext(w, r, log))
		})
	}
}

func (r *govrouter) GroupCtx(path string, mw ...MiddlewareCtx) Router {
	cpath := r.path + path
	l := r.log.Sublogger("", klog.Fields{
		"gov.router.path": cpath,
	})
	sr := r.r.Route(path, func(r chi.Router) {})
	if len(mw) > 0 {
		sr = sr.With(r.toHTTPMiddleware(mw, l))
	}
	return &govrouter{
		r:    sr,
		log:  l,
		path: cpath,
	}
}

func (r *govrouter) toHTTPHandler(fn HandlerFunc, log klog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fn(NewContext(w, r, log))
	}
}

func (r *govrouter) methodCtx(method string, path string, fn HandlerFunc, mw []MiddlewareCtx) {
	l := r.log.Sublogger("", klog.Fields{
		"gov.router.path": r.path + path,
	})
	if path == "" {
		path = "/"
	}
	fn = r.chainMiddlewareCtx(fn, mw)
	r.r.MethodFunc(method, path, r.toHTTPHandler(fn, l))
}

func (r *govrouter) MethodCtx(method string, path string, fn HandlerFunc, mw ...MiddlewareCtx) {
	r.methodCtx(method, path, fn, mw)
}

func (r *govrouter) GetCtx(path string, fn HandlerFunc, mw ...MiddlewareCtx) {
	r.methodCtx(http.MethodGet, path, fn, mw)
}

func (r *govrouter) PostCtx(path string, fn HandlerFunc, mw ...MiddlewareCtx) {
	r.methodCtx(http.MethodPost, path, fn, mw)
}

func (r *govrouter) PutCtx(path string, fn HandlerFunc, mw ...MiddlewareCtx) {
	r.methodCtx(http.MethodPut, path, fn, mw)
}

func (r *govrouter) PatchCtx(path string, fn HandlerFunc, mw ...MiddlewareCtx) {
	r.methodCtx(http.MethodPatch, path, fn, mw)
}

func (r *govrouter) DeleteCtx(path string, fn HandlerFunc, mw ...MiddlewareCtx) {
	r.methodCtx(http.MethodDelete, path, fn, mw)
}

func (r *govrouter) AnyCtx(path string, fn HandlerFunc, mw ...MiddlewareCtx) {
	l := r.log.Sublogger("", klog.Fields{
		"gov.router.path": r.path + path,
	})
	if path == "" {
		path = "/"
	}
	fn = r.chainMiddlewareCtx(fn, mw)
	r.r.HandleFunc(path, r.toHTTPHandler(fn, l))
}

func (r *govrouter) NotFoundCtx(fn HandlerFunc, mw ...MiddlewareCtx) {
	l := r.log.Sublogger("", klog.Fields{
		"gov.router.notfound": true,
	})
	fn = r.chainMiddlewareCtx(fn, mw)
	r.r.NotFound(r.toHTTPHandler(fn, l))
}

func (r *govrouter) MethodNotAllowedCtx(fn HandlerFunc, mw ...MiddlewareCtx) {
	l := r.log.Sublogger("", klog.Fields{
		"gov.router.methodnotallowed": true,
	})
	fn = r.chainMiddlewareCtx(fn, mw)
	r.r.MethodNotAllowed(r.toHTTPHandler(fn, l))
}

type (
	// Context is an http request and writer wrapper
	Context interface {
		LReqID() string
		RealIP() net.IP
		Param(key string) string
		Query(key string) string
		QueryDef(key string, def string) string
		QueryInt(key string, def int) int
		QueryInt64(key string, def int64) int64
		QueryBool(key string) bool
		Header(key string) string
		SetHeader(key, value string)
		AddHeader(key, value string)
		Cookie(key string) (*http.Cookie, error)
		SetCookie(cookie *http.Cookie)
		BasicAuth() (string, string, bool)
		ReadAllBody() ([]byte, error)
		Bind(i interface{}) error
		FormValue(key string) string
		FormFile(key string) (multipart.File, *multipart.FileHeader, error)
		WriteStatus(status int)
		Redirect(status int, url string)
		WriteString(status int, text string)
		WriteJSON(status int, body interface{})
		WriteFile(status int, contentType string, r io.Reader)
		WriteError(err error)
		Ctx() context.Context
		SetCtx(ctx context.Context)
		Get(key interface{}) interface{}
		Set(key, value interface{})
		LogFields(fields klog.Fields)
		Req() *http.Request
		Res() http.ResponseWriter
		R() (http.ResponseWriter, *http.Request)
		Websocket() (Websocket, error)
	}

	govcontext struct {
		w        http.ResponseWriter
		r        *http.Request
		query    url.Values
		rawquery string
		log      *klog.LevelLogger
	}
)

// NewContext creates a Context
func NewContext(w http.ResponseWriter, r *http.Request, log klog.Logger) Context {
	return &govcontext{
		w:        w,
		r:        r,
		query:    r.URL.Query(),
		rawquery: r.URL.RawQuery,
		log:      klog.NewLevelLogger(log),
	}
}

func (c *govcontext) LReqID() string {
	return getCtxLocalReqID(c.Ctx())
}

func (c *govcontext) RealIP() net.IP {
	if ip := getCtxMiddlewareRealIP(c.Ctx()); ip != nil {
		return ip
	}
	host, _, err := net.SplitHostPort(c.Req().RemoteAddr)
	if err != nil {
		return nil
	}
	return net.ParseIP(host)
}

func (c *govcontext) Param(key string) string {
	return chi.URLParam(c.Req(), key)
}

func (c *govcontext) Query(key string) string {
	if u := c.Req().URL; u.RawQuery != c.rawquery {
		c.query = u.Query()
		c.rawquery = u.RawQuery
	}
	return c.query.Get(key)
}

func (c *govcontext) QueryDef(key string, def string) string {
	v := c.Query(key)
	if v == "" {
		return def
	}
	return v
}

func (c *govcontext) QueryInt(key string, def int) int {
	s := c.Query(key)
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

func (c *govcontext) QueryInt64(key string, def int64) int64 {
	s := c.Query(key)
	if s == "" {
		return def
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return def
	}
	return v
}

func (c *govcontext) QueryBool(key string) bool {
	s := c.Query(key)
	switch s {
	case "t", "true", "y", "yes", "1":
		return true
	default:
		return false
	}
}

func (c *govcontext) Header(key string) string {
	return c.Req().Header.Get(key)
}

func (c *govcontext) SetHeader(key, value string) {
	c.w.Header().Set(key, value)
}

func (c *govcontext) AddHeader(key, value string) {
	c.w.Header().Add(key, value)
}

func (c *govcontext) Cookie(key string) (*http.Cookie, error) {
	return c.Req().Cookie(key)
}

func (c *govcontext) SetCookie(cookie *http.Cookie) {
	http.SetCookie(c.w, cookie)
}

func (c *govcontext) BasicAuth() (string, string, bool) {
	return c.Req().BasicAuth()
}

func (c *govcontext) ReadAllBody() ([]byte, error) {
	data, err := io.ReadAll(c.Req().Body)
	if err != nil {
		var rerr *http.MaxBytesError
		if errors.As(err, &rerr) {
			return nil, ErrWithRes(err, http.StatusRequestEntityTooLarge, "", "Request too large")
		}
		return nil, ErrWithRes(err, http.StatusBadRequest, "", "Failed reading request")
	}
	return data, nil
}

func (c *govcontext) Bind(i interface{}) error {
	// ContentLength of -1 is unknown
	if c.Req().ContentLength == 0 {
		return ErrWithRes(nil, http.StatusBadRequest, "", "Empty request body")
	}
	mediaType, _, err := mime.ParseMediaType(c.Req().Header.Get("Content-Type"))
	if err != nil {
		return ErrWithRes(err, http.StatusBadRequest, "", "Invalid mime type")
	}
	switch mediaType {
	case "application/json":
		data, err := c.ReadAllBody()
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, i); err != nil {
			return ErrWithRes(err, http.StatusBadRequest, "", "Invalid JSON")
		}
	default:
		return ErrWithRes(nil, http.StatusUnsupportedMediaType, "", "Unsupported media type")
	}
	return nil
}

func (c *govcontext) FormValue(key string) string {
	return c.Req().FormValue(key)
}

func (c *govcontext) FormFile(key string) (multipart.File, *multipart.FileHeader, error) {
	return c.Req().FormFile(key)
}

func (c *govcontext) WriteStatus(status int) {
	c.w.WriteHeader(status)
}

func (c *govcontext) Redirect(status int, url string) {
	http.Redirect(c.Res(), c.Req(), url, status)
}

func (c *govcontext) WriteString(status int, text string) {
	c.w.Header().Set("Content-Type", mime.FormatMediaType("text/plain", map[string]string{"charset": "utf-8"}))
	c.w.WriteHeader(status)
	if _, err := io.WriteString(c.w, text); err != nil {
		c.log.Err(c.Ctx(), kerrors.WithMsg(err, "Failed to write string bytes"), nil)
	}
}

func (c *govcontext) WriteJSON(status int, body interface{}) {
	b := bytes.Buffer{}
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(body); err != nil {
		c.log.Err(c.Ctx(), kerrors.WithMsg(err, "Failed to write json"), nil)
		http.Error(c.w, "Failed to write response", http.StatusInternalServerError)
		return
	}

	c.w.Header().Set("Content-Type", mime.FormatMediaType("application/json", map[string]string{"charset": "utf-8"}))
	c.w.WriteHeader(status)
	if _, err := io.Copy(c.w, &b); err != nil {
		c.log.Err(c.Ctx(), kerrors.WithMsg(err, "Failed to write json bytes"), nil)
	}
}

func (c *govcontext) WriteFile(status int, contentType string, r io.Reader) {
	c.w.Header().Set("Content-Type", contentType)
	c.w.WriteHeader(status)
	if _, err := io.Copy(c.w, r); err != nil {
		c.log.Err(c.Ctx(), kerrors.WithMsg(err, "Failed to write file"), nil)
		return
	}
}

func (c *govcontext) Ctx() context.Context {
	return c.r.Context()
}

func (c *govcontext) SetCtx(ctx context.Context) {
	c.r = c.r.WithContext(ctx)
}

func (c *govcontext) Get(key interface{}) interface{} {
	return c.Ctx().Value(key)
}

func (c *govcontext) Set(key, value interface{}) {
	c.SetCtx(context.WithValue(c.Ctx(), key, value))
}

func (c *govcontext) LogFields(fields klog.Fields) {
	c.SetCtx(klog.WithFields(c.Ctx(), fields))
}

func (c *govcontext) Req() *http.Request {
	return c.r
}

func (c *govcontext) Res() http.ResponseWriter {
	return c.w
}

func (c *govcontext) R() (http.ResponseWriter, *http.Request) {
	return c.Res(), c.Req()
}

type (
	Websocket interface {
		SetReadLimit(limit int64)
		Read(ctx context.Context) (bool, []byte, error)
		Write(ctx context.Context, txt bool, b []byte) error
		Close(status int, reason string)
		CloseError(err error)
	}

	govws struct {
		c    *govcontext
		conn *websocket.Conn
	}
)

func (c *govcontext) Websocket() (Websocket, error) {
	conn, err := websocket.Accept(c.Res(), c.Req(), &websocket.AcceptOptions{
		Subprotocols:    []string{WSProtocolVersion},
		CompressionMode: websocket.CompressionContextTakeover,
	})
	if err != nil {
		return nil, ErrWithRes(err, http.StatusBadRequest, "", "Failed to open ws connection")
	}
	w := &govws{
		c:    c,
		conn: conn,
	}
	if conn.Subprotocol() != WSProtocolVersion {
		w.Close(int(websocket.StatusPolicyViolation), "Invalid ws subprotocol")
		return nil, ErrWithRes(nil, http.StatusBadRequest, "", "Invalid ws subprotocol")
	}
	w.SetReadLimit(WSReadLimitDefault)
	return w, nil
}

func (w *govws) SetReadLimit(limit int64) {
	w.conn.SetReadLimit(limit)
}

type (
	ErrorWS struct {
		Status int
		Reason string
	}
)

func (e *ErrorWS) Error() string {
	b := strings.Builder{}
	b.WriteString("(")
	b.WriteString(strconv.Itoa(e.Status))
	b.WriteString(")")
	if e.Reason != "" {
		b.WriteString(" ")
		b.WriteString(e.Reason)
	}
	return b.String()
}

func (w *govws) wrapWSErr(err error, status int, reason string) error {
	var werr websocket.CloseError
	if errors.As(err, &werr) {
		return kerrors.WithKind(err, &ErrorWS{
			Status: int(werr.Code),
			Reason: werr.Reason,
		}, "Websocket error")
	}
	return kerrors.WithKind(err, &ErrorWS{
		Status: status,
		Reason: reason,
	}, "Websocket error")
}

// ErrWS returns a wrapped error with a websocket code
func ErrWS(err error, status int, reason string) error {
	return kerrors.WithKind(err, &ErrorWS{
		Status: status,
		Reason: reason,
	}, "Websocket error")
}

func (w *govws) Read(ctx context.Context) (bool, []byte, error) {
	t, b, err := w.conn.Read(ctx)
	if err != nil {
		return false, nil, w.wrapWSErr(err, int(websocket.StatusUnsupportedData), "Failed to read from ws")
	}
	return t == websocket.MessageText, b, nil
}

func (w *govws) Write(ctx context.Context, txt bool, b []byte) error {
	msgtype := websocket.MessageBinary
	if txt {
		msgtype = websocket.MessageText
	}
	reqctx, reqcancel := context.WithTimeout(ctx, 5*time.Second)
	defer reqcancel()
	if err := w.conn.Write(reqctx, msgtype, b); err != nil {
		return w.wrapWSErr(err, int(websocket.StatusInternalError), "Failed to write to ws")
	}
	return nil
}

func (w *govws) Close(status int, reason string) {
	if err := w.conn.Close(websocket.StatusCode(status), reason); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "already wrote close") {
			return
		}
		err = w.wrapWSErr(err, int(websocket.StatusInternalError), "Failed to close ws connection")
		w.c.log.WarnErr(w.c.Ctx(), kerrors.WithMsg(err, "Failed to close ws connection"), nil)
	}
}

func (w *govws) CloseError(err error) {
	var werr *ErrorWS
	isError := errors.As(err, &werr)
	if !isError {
		werr = &ErrorWS{
			Status: int(websocket.StatusInternalError),
			Reason: "Internal error",
		}
	}

	if !errors.Is(err, ErrorNoLog{}) {
		if werr.Status != int(websocket.StatusInternalError) {
			w.c.log.WarnErr(w.c.Ctx(), err, nil)
		} else {
			w.c.log.Err(w.c.Ctx(), err, nil)
		}
	}

	w.Close(werr.Status, werr.Reason)
}

func (s *Server) bodyLimitMiddleware(limit int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// ContentLength of -1 is unknown
			if r.ContentLength > limit {
				c := NewContext(w, r, s.log.Logger)
				c.WriteError(ErrWithRes(nil, http.StatusRequestEntityTooLarge, "", "Request too large"))
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}

func stripSlashesMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r2 := new(http.Request)
		*r2 = *r
		r2.URL = new(url.URL)
		*r2.URL = *r.URL
		path := r2.URL.Path
		if l := len(path); l > 1 && path[l-1] == '/' {
			r2.URL.Path = path[:l-1]
		}
		next.ServeHTTP(w, r)
	})
}

func corsPathsAllowAllMiddleware(rules []*corsPathRule) Middleware {
	allowAll := cors.AllowAll()
	return func(next http.Handler) http.Handler {
		corsNext := allowAll.Handler(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			isMatch := false
			for _, i := range rules {
				if i.match(r) {
					isMatch = true
					break
				}
			}
			if isMatch {
				corsNext.ServeHTTP(w, r)
			} else {
				next.ServeHTTP(w, r)
			}
		})
	}
}

func routeRewriteMiddleware(rules []*rewriteRule) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r2 := new(http.Request)
			*r2 = *r
			r2.URL = new(url.URL)
			*r2.URL = *r.URL
			for _, i := range rules {
				if i.match(r2) {
					r2.URL.Path = i.replace(r2.URL.Path)
				}
			}
			next.ServeHTTP(w, r2)
		})
	}
}

const (
	headerXForwardedFor = "X-Forwarded-For"
)

type (
	ctxKeyMiddlewareRealIP struct{}
)

func realIPMiddleware(proxies []net.IPNet) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			ip := getForwardedForIP(r, proxies)
			if ip != nil {
				ctx = context.WithValue(ctx, ctxKeyMiddlewareRealIP{}, ip)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func getCtxMiddlewareRealIP(ctx context.Context) net.IP {
	k := ctx.Value(ctxKeyMiddlewareRealIP{})
	if k == nil {
		return nil
	}
	return k.(net.IP)
}

func getForwardedForIP(r *http.Request, proxies []net.IPNet) net.IP {
	xff := r.Header.Get(headerXForwardedFor)
	if xff == "" {
		return nil
	}

	ipstrs := strings.Split(xff, ",")
	for i := len(ipstrs) - 1; i >= 0; i-- {
		ip := net.ParseIP(strings.TrimSpace(ipstrs[i]))
		if ip == nil {
			break
		}
		if !ipnetsContain(ip, proxies) {
			return ip
		}
	}

	return nil
}

func ipnetsContain(ip net.IP, ipnet []net.IPNet) bool {
	for _, i := range ipnet {
		if i.Contains(ip) {
			return true
		}
	}
	return false
}

const (
	headerConnection           = "Connection"
	headerUpgrade              = "Upgrade"
	headerConnectionValUpgrade = "upgrade"
	headerUpgradeValWS         = "websocket"
)

func reqIsWS(r *http.Request) bool {
	isUpgrade := false
	for _, i := range r.Header.Values(headerConnection) {
		if strings.Contains(strings.ToLower(i), headerConnectionValUpgrade) {
			isUpgrade = true
			break
		}
	}
	if !isUpgrade {
		return false
	}
	for _, i := range r.Header.Values(headerUpgrade) {
		if strings.Contains(strings.ToLower(i), headerUpgradeValWS) {
			return true
		}
	}
	return false
}

func compressorMiddleware(next http.Handler) http.Handler {
	compressor := middleware.Compress(gzip.DefaultCompression)(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reqIsWS(r) {
			next.ServeHTTP(w, r)
		} else {
			compressor.ServeHTTP(w, r)
		}
	})
}

func (s *Server) recovererMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := NewContext(w, r, s.log.Logger)
		defer func() {
			if re := recover(); re != nil {
				if re == http.ErrAbortHandler {
					// may not recover http.ErrAbortHandler so re-panic the error
					panic(re)
				}

				s.log.Error(r.Context(), "Panicked in http handler", klog.Fields{
					"recovered":  re,
					"stacktrace": debug.Stack(),
				})

				c.WriteError(ErrWithRes(kerrors.WithMsg(nil, "Panicked in http handler"), http.StatusInternalServerError, "", "Internal Server Error"))
			}
		}()
		next.ServeHTTP(c.R())
	})
}
