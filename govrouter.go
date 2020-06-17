package governor

import (
	"bytes"
	"encoding/json"
	"github.com/go-chi/chi"
	"mime"
	"net/http"
)

type (
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
	Context interface {
		Bind(i interface{}) error
		WriteStatus(status int)
		WriteString(status int, text string)
		WriteJSON(status int, body interface{})
		WriteError(err error)
	}

	govcontext struct {
		w http.ResponseWriter
		r *http.Request
		l Logger
	}
)

func NewContext(w http.ResponseWriter, r *http.Request, l Logger) Context {
	return &govcontext{
		w: w,
		r: r,
		l: l,
	}
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
		if err := json.NewDecoder(c.r.Body).Decode(i); err != nil {
			return NewErrorUser("Invalid JSON", http.StatusBadRequest, err)
		}
	default:
		return NewErrorUser("Unsupported media type", http.StatusUnsupportedMediaType, nil)
	}
	return nil
}

func (c *govcontext) WriteStatus(status int) {
	c.w.WriteHeader(status)
}

func (c *govcontext) WriteString(status int, text string) {
	c.w.Header().Set("Content-Type", mime.FormatMediaType("text/plain", map[string]string{"charset": "utf-8"}))
	c.w.WriteHeader(status)
	c.w.Write([]byte(text))
}

func (c *govcontext) WriteJSON(status int, body interface{}) {
	b := &bytes.Buffer{}
	e := json.NewEncoder(b)
	e.SetEscapeHTML(false)
	if err := e.Encode(body); err != nil {
		if c.l != nil {
			c.l.Error("failed to write json", map[string]string{
				"endpoint": c.r.URL.EscapedPath(),
				"error":    err.Error(),
			})
		}
		http.Error(c.w, "Failed to write response", http.StatusInternalServerError)
		return
	}

	c.w.Header().Set("Content-Type", mime.FormatMediaType("application/json", map[string]string{"charset": "utf-8"}))
	c.w.WriteHeader(status)
	c.w.Write(b.Bytes())
}

func (s *Server) bodyLimitMiddleware(limit int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > limit {
				c := NewContext(w, r, s.logger)
				c.WriteError(NewErrorUser("Request too large", http.StatusRequestEntityTooLarge, nil))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
