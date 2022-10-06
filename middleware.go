package governor

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"

	"github.com/go-chi/cors"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

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
	headerContentEncoding = "Content-Encoding"
)

type (
	compressorWriter struct {
		http.ResponseWriter
		r            *http.Request
		status       int
		compressable bool
		wroteHeader  bool
	}
)

func (w *compressorWriter) shouldCompress() bool {
	if w.ResponseWriter.Header().Get(headerContentEncoding) != "" {
		// do not re-compress compressed data
		return false
	}
	if w.status == http.StatusSwitchingProtocols {
		// do not compress switched protocols, e.g. websockets
		return false
	}
	// TODO check accept encoding headers and response content type headers
	return false
}

func (w *compressorWriter) WriteHeader(status int) {
	if w.wroteHeader {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	w.status = status
	w.compressable = w.shouldCompress()
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *compressorWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if !w.compressable {
		return w.ResponseWriter.Write(p)
	}
	// TODO use compress writer
	return w.ResponseWriter.Write(p)
}

func compressorMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w2 := &compressorWriter{
			ResponseWriter: w,
			r:              r,
			status:         0,
			compressable:   false,
			wroteHeader:    false,
		}
		next.ServeHTTP(w2, r)
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
