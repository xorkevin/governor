package governor

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi"
)

type (
	// Router adds route handlers to the server
	Router interface {
		Group(path string) Router
		Get(path string, fn http.HandlerFunc, mw ...Middleware)
		Post(path string, fn http.HandlerFunc, mw ...Middleware)
		Put(path string, fn http.HandlerFunc, mw ...Middleware)
		Patch(path string, fn http.HandlerFunc, mw ...Middleware)
		Delete(path string, fn http.HandlerFunc, mw ...Middleware)
		Any(path string, fn http.HandlerFunc, mw ...Middleware)
	}

	govrouter struct {
		r chi.Router
	}

	// Middleware is a type alias for Router middleware
	Middleware = func(next http.Handler) http.Handler
)

func (s *Server) router(path string) Router {
	return &govrouter{
		r: s.i.Route(path, nil),
	}
}

func (r *govrouter) Group(path string) Router {
	return &govrouter{
		r: r.r.Route(path, nil),
	}
}

func (r *govrouter) Get(path string, fn http.HandlerFunc, mw ...Middleware) {
	if path == "" {
		path = "/"
	}
	k := r.r
	if l := len(mw); l > 0 {
		k = r.r.With(mw...)
	}
	k.Get(path, fn)
}

func (r *govrouter) Post(path string, fn http.HandlerFunc, mw ...Middleware) {
	if path == "" {
		path = "/"
	}
	k := r.r
	if l := len(mw); l > 0 {
		k = r.r.With(mw...)
	}
	k.Post(path, fn)
}

func (r *govrouter) Put(path string, fn http.HandlerFunc, mw ...Middleware) {
	if path == "" {
		path = "/"
	}
	k := r.r
	if l := len(mw); l > 0 {
		k = r.r.With(mw...)
	}
	k.Put(path, fn)
}

func (r *govrouter) Patch(path string, fn http.HandlerFunc, mw ...Middleware) {
	if path == "" {
		path = "/"
	}
	k := r.r
	if l := len(mw); l > 0 {
		k = r.r.With(mw...)
	}
	k.Patch(path, fn)
}

func (r *govrouter) Delete(path string, fn http.HandlerFunc, mw ...Middleware) {
	if path == "" {
		path = "/"
	}
	k := r.r
	if l := len(mw); l > 0 {
		k = r.r.With(mw...)
	}
	k.Delete(path, fn)
}

func (r *govrouter) Any(path string, fn http.HandlerFunc, mw ...Middleware) {
	if path == "" {
		path = "/"
	}
	k := r.r
	if l := len(mw); l > 0 {
		k = r.r.With(mw...)
	}
	k.HandleFunc(path, fn)
}

type (
	// Context is an http request and writer wrapper
	Context interface {
		Param(key string) string
		Query(key string) string
		QueryDef(key string, def string) string
		QueryInt(key string, def int) int
		Header(key string) string
		SetHeader(key, value string)
		AddHeader(key, value string)
		Cookie(key string) (*http.Cookie, error)
		SetCookie(cookie *http.Cookie)
		Bind(i interface{}) error
		FormValue(key string) string
		FormFile(key string) (multipart.File, *multipart.FileHeader, error)
		WriteStatus(status int)
		Redirect(status int, url string)
		WriteString(status int, text string)
		WriteJSON(status int, body interface{})
		WriteFile(status int, contentType string, r io.Reader)
		WriteError(err error)
		Get(key interface{}) interface{}
		Set(key, value interface{})
		Req() *http.Request
		Res() http.ResponseWriter
		R() (http.ResponseWriter, *http.Request)
	}

	govcontext struct {
		w     http.ResponseWriter
		r     *http.Request
		query url.Values
		ctx   context.Context
		l     Logger
	}
)

// NewContext creates a Context
func NewContext(w http.ResponseWriter, r *http.Request, l Logger) Context {
	return &govcontext{
		w:     w,
		r:     r,
		query: r.URL.Query(),
		l:     l,
	}
}

func (c *govcontext) Param(key string) string {
	return chi.URLParam(c.r, key)
}

func (c *govcontext) Query(key string) string {
	return c.query.Get(key)
}

func (c *govcontext) QueryDef(key string, def string) string {
	v := c.query.Get(key)
	if v == "" {
		return def
	}
	return v
}

func (c *govcontext) QueryInt(key string, def int) int {
	s := c.query.Get(key)
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

func (c *govcontext) Header(key string) string {
	return c.r.Header.Get(key)
}

func (c *govcontext) SetHeader(key, value string) {
	c.w.Header().Set(key, value)
}

func (c *govcontext) AddHeader(key, value string) {
	c.w.Header().Add(key, value)
}

func (c *govcontext) Cookie(key string) (*http.Cookie, error) {
	return c.r.Cookie(key)
}

func (c *govcontext) SetCookie(cookie *http.Cookie) {
	http.SetCookie(c.w, cookie)
}

func (c *govcontext) Bind(i interface{}) error {
	if c.r.ContentLength == 0 {
		return NewErrorUser("Empty request body", http.StatusBadRequest, nil)
	}
	mediaType, _, err := mime.ParseMediaType(c.r.Header.Get("Content-Type"))
	if err != nil {
		return NewErrorUser("Invalid mime type", http.StatusBadRequest, err)
	}
	switch mediaType {
	case "application/json":
		data, err := ioutil.ReadAll(c.r.Body)
		if err != nil {
			if err.Error() == "http: request body too large" {
				return NewErrorUser("Request too large", http.StatusRequestEntityTooLarge, err)
			}
			return NewErrorUser("Failed reading request", http.StatusBadRequest, err)
		}
		if err := json.Unmarshal(data, i); err != nil {
			return NewErrorUser("Invalid JSON", http.StatusBadRequest, err)
		}
	default:
		return NewErrorUser("Unsupported media type", http.StatusUnsupportedMediaType, nil)
	}
	return nil
}

func (c *govcontext) FormValue(key string) string {
	return c.r.FormValue(key)
}

func (c *govcontext) FormFile(key string) (multipart.File, *multipart.FileHeader, error) {
	return c.r.FormFile(key)
}

func (c *govcontext) WriteStatus(status int) {
	c.w.WriteHeader(status)
}

func (c *govcontext) Redirect(status int, url string) {
	http.Redirect(c.w, c.r, url, status)
}

func (c *govcontext) WriteString(status int, text string) {
	c.w.Header().Set("Content-Type", mime.FormatMediaType("text/plain", map[string]string{"charset": "utf-8"}))
	c.w.WriteHeader(status)
	if _, err := c.w.Write([]byte(text)); err != nil {
		if c.l != nil {
			c.l.Error("Failed to write string bytes", map[string]string{
				"endpoint": c.r.URL.EscapedPath(),
				"error":    err.Error(),
			})
		}
	}
}

func (c *govcontext) WriteJSON(status int, body interface{}) {
	b := &bytes.Buffer{}
	e := json.NewEncoder(b)
	e.SetEscapeHTML(false)
	if err := e.Encode(body); err != nil {
		if c.l != nil {
			c.l.Error("Failed to write json", map[string]string{
				"endpoint": c.r.URL.EscapedPath(),
				"error":    err.Error(),
			})
		}
		http.Error(c.w, "Failed to write response", http.StatusInternalServerError)
		return
	}

	c.w.Header().Set("Content-Type", mime.FormatMediaType("application/json", map[string]string{"charset": "utf-8"}))
	c.w.WriteHeader(status)
	if _, err := c.w.Write(b.Bytes()); err != nil {
		if c.l != nil {
			c.l.Error("Failed to write json bytes", map[string]string{
				"endpoint": c.r.URL.EscapedPath(),
				"error":    err.Error(),
			})
		}
	}
}

func (c *govcontext) WriteFile(status int, contentType string, r io.Reader) {
	c.w.Header().Set("Content-Type", contentType)
	c.w.WriteHeader(status)
	if _, err := io.Copy(c.w, r); err != nil {
		if c.l != nil {
			c.l.Error("Failed to write file", map[string]string{
				"endpoint": c.r.URL.EscapedPath(),
				"error":    err.Error(),
			})
		}
		return
	}
}

func (c *govcontext) Get(key interface{}) interface{} {
	return c.r.Context().Value(key)
}

func (c *govcontext) Set(key, value interface{}) {
	if c.ctx == nil {
		c.ctx = c.r.Context()
	}
	c.ctx = context.WithValue(c.ctx, key, value)
}

func (c *govcontext) Req() *http.Request {
	if c.ctx != nil {
		return c.r.WithContext(c.ctx)
	}
	return c.r
}

func (c *govcontext) Res() http.ResponseWriter {
	return c.w
}

func (c *govcontext) R() (http.ResponseWriter, *http.Request) {
	return c.Res(), c.Req()
}

func (s *Server) bodyLimitMiddleware(limit int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > limit {
				c := NewContext(w, r, s.logger)
				c.WriteError(NewErrorUser("Request too large", http.StatusRequestEntityTooLarge, nil))
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
