package governor

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"xorkevin.dev/klog"
)

type (
	// Handler responds to an HTTP request with Context
	Handler interface {
		ServeHTTPCtx(c Context)
	}

	// HandlerFunc adapts a function as a [Handler]
	HandlerFunc func(c Context)

	// Middleware is a type alias for [Router] middleware
	Middleware = func(next http.Handler) http.Handler

	// MiddlewareCtx is a type alias for [Router] middleware with Context
	MiddlewareCtx = func(next Handler) Handler
)

// ServeHTTPCtx implements [Handler]
func (f HandlerFunc) ServeHTTPCtx(c Context) {
	f(c)
}

type (
	// Router adds route handlers to the server
	Router interface {
		Group(path string, mw ...Middleware) Router
		Method(method string, path string, fn http.Handler, mw ...Middleware)
		NotFound(fn http.Handler, mw ...Middleware)
		MethodNotAllowed(fn http.Handler, mw ...Middleware)

		GroupCtx(path string, mw ...MiddlewareCtx) Router
		MethodCtx(method string, path string, fn Handler, mw ...MiddlewareCtx)
		NotFoundCtx(fn Handler, mw ...MiddlewareCtx)
		MethodNotAllowedCtx(fn Handler, mw ...MiddlewareCtx)
	}

	govrouter struct {
		r    chi.Router
		log  klog.Logger
		path string
	}
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

func (r *govrouter) Method(method string, path string, h http.Handler, mw ...Middleware) {
	if path == "" {
		path = "/"
	}
	k := r.r
	if len(mw) > 0 {
		k = r.r.With(mw...)
	}
	if method == "" {
		k.Handle(path, h)
	} else {
		k.Method(method, path, h)
	}
}

func toHTTPHandlerFunc(h http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r)
	}
}

func (r *govrouter) NotFound(h http.Handler, mw ...Middleware) {
	k := r.r
	if len(mw) > 0 {
		k = r.r.With(mw...)
	}
	k.NotFound(toHTTPHandlerFunc(h))
}

func (r *govrouter) MethodNotAllowed(h http.Handler, mw ...Middleware) {
	k := r.r
	if len(mw) > 0 {
		k = r.r.With(mw...)
	}
	k.MethodNotAllowed(toHTTPHandlerFunc(h))
}

func toHTTPHandler(h Handler, log klog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTPCtx(NewContext(w, r, log))
	})
}

func chainMiddlewareCtx(h Handler, mw []MiddlewareCtx) Handler {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}

func toMiddleware(mw []MiddlewareCtx, log klog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return toHTTPHandler(chainMiddlewareCtx(HandlerFunc(func(c Context) {
			next.ServeHTTP(c.Res(), c.Req())
		}), mw), log)
	}
}

func (r *govrouter) GroupCtx(path string, mw ...MiddlewareCtx) Router {
	cpath := r.path + path
	l := r.log.Sublogger("", klog.Fields{
		"gov.router.path": cpath,
	})
	sr := r.r.Route(path, func(r chi.Router) {})
	if len(mw) > 0 {
		sr = sr.With(toMiddleware(mw, l))
	}
	return &govrouter{
		r:    sr,
		log:  l,
		path: cpath,
	}
}

func (r *govrouter) MethodCtx(method string, path string, h Handler, mw ...MiddlewareCtx) {
	lmethod := method
	if lmethod == "" {
		lmethod = "ANY"
	}
	r.Method(method, path, toHTTPHandler(chainMiddlewareCtx(h, mw), r.log.Sublogger("", klog.Fields{
		"gov.router.method": lmethod,
		"gov.router.path":   r.path + path,
	})))
}

func (r *govrouter) NotFoundCtx(h Handler, mw ...MiddlewareCtx) {
	r.NotFound(toHTTPHandler(chainMiddlewareCtx(h, mw), r.log.Sublogger("", klog.Fields{
		"gov.router.notfound": true,
	})))
}

func (r *govrouter) MethodNotAllowedCtx(h Handler, mw ...MiddlewareCtx) {
	r.MethodNotAllowed(toHTTPHandler(chainMiddlewareCtx(h, mw), r.log.Sublogger("", klog.Fields{
		"gov.router.methodnotallowed": true,
	})))
}

type (
	// MethodRouter routes by HTTP method
	MethodRouter struct {
		Router Router
	}
)

// NewMethodRouter creates a new [*MethodRouter]
func NewMethodRouter(r Router) *MethodRouter {
	return &MethodRouter{
		Router: r,
	}
}

func (r *MethodRouter) Get(path string, fn http.HandlerFunc, mw ...Middleware) {
	r.Router.Method(http.MethodGet, path, fn, mw...)
}

func (r *MethodRouter) Post(path string, fn http.HandlerFunc, mw ...Middleware) {
	r.Router.Method(http.MethodPost, path, fn, mw...)
}

func (r *MethodRouter) Put(path string, fn http.HandlerFunc, mw ...Middleware) {
	r.Router.Method(http.MethodPut, path, fn, mw...)
}

func (r *MethodRouter) Patch(path string, fn http.HandlerFunc, mw ...Middleware) {
	r.Router.Method(http.MethodPatch, path, fn, mw...)
}

func (r *MethodRouter) Delete(path string, fn http.HandlerFunc, mw ...Middleware) {
	r.Router.Method(http.MethodDelete, path, fn, mw...)
}

func (r *MethodRouter) Any(path string, fn http.HandlerFunc, mw ...Middleware) {
	r.Router.Method("", path, fn, mw...)
}

func (r *MethodRouter) GetCtx(path string, fn HandlerFunc, mw ...MiddlewareCtx) {
	r.Router.MethodCtx(http.MethodGet, path, fn, mw...)
}

func (r *MethodRouter) PostCtx(path string, fn HandlerFunc, mw ...MiddlewareCtx) {
	r.Router.MethodCtx(http.MethodPost, path, fn, mw...)
}

func (r *MethodRouter) PutCtx(path string, fn HandlerFunc, mw ...MiddlewareCtx) {
	r.Router.MethodCtx(http.MethodPut, path, fn, mw...)
}

func (r *MethodRouter) PatchCtx(path string, fn HandlerFunc, mw ...MiddlewareCtx) {
	r.Router.MethodCtx(http.MethodPatch, path, fn, mw...)
}

func (r *MethodRouter) DeleteCtx(path string, fn HandlerFunc, mw ...MiddlewareCtx) {
	r.Router.MethodCtx(http.MethodDelete, path, fn, mw...)
}

func (r *MethodRouter) AnyCtx(path string, fn HandlerFunc, mw ...MiddlewareCtx) {
	r.Router.MethodCtx("", path, fn, mw...)
}
