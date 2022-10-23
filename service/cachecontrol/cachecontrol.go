package cachecontrol

import (
	"fmt"
	"net/http"
	"strings"

	"xorkevin.dev/governor"
	"xorkevin.dev/klog"
)

const (
	headerCacheControl = "Cache-Control"
	headerIfNoneMatch  = "If-None-Match"
	headerETag         = "ETag"
)

type (
	// Directive is a cache control directive
	Directive string

	//Directives are a list of directives
	Directives []Directive
)

// Cache control directives
const (
	// Cacheability
	DirPublic  Directive = "public"
	DirPrivate Directive = "private"
	DirNoStore Directive = "no-store"
	// Expiration
	DirMaxAge Directive = "max-age"
	// Revalidation
	DirNoCache        Directive = "no-cache"
	DirMustRevalidate Directive = "must-revalidate"
	DirImmutable      Directive = "immutable"
)

type (
	cacheControlWriter struct {
		http.ResponseWriter
		valCC       string
		valETag     string
		wroteHeader bool
	}
)

func (w *cacheControlWriter) shouldAddHeaders(status int) bool {
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return false
	}
	if w.ResponseWriter.Header().Get(headerCacheControl) != "" {
		return false
	}
	return w.valCC != ""
}

func (w *cacheControlWriter) WriteHeader(status int) {
	if w.wroteHeader {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	if w.shouldAddHeaders(status) {
		w.ResponseWriter.Header().Set(headerCacheControl, w.valCC)
		if w.valETag != "" {
			w.ResponseWriter.Header().Set(headerETag, w.valETag)
		}
	}
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *cacheControlWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}

func etagToValue(etag string) string {
	return fmt.Sprintf(`W/"%s"`, etag)
}

// ControlCtx creates a middleware function to cache the response
func ControlCtx(public bool, directives Directives, maxage int64, etagfunc func(governor.Context) (string, error)) governor.MiddlewareCtx {
	return func(next governor.RouteHandler) governor.RouteHandler {
		return governor.RouteHandlerFunc(func(c governor.Context) {
			etag := ""
			if etagfunc != nil {
				tag, err := etagfunc(c)
				if err != nil {
					c.WriteError(err)
					return
				}
				etag = etagToValue(tag)
			}

			if val := c.Header(headerIfNoneMatch); etag != "" && val == etag {
				c.WriteStatus(http.StatusNotModified)
				return
			}

			finalDirectives := make([]string, 0, 2+len(directives))
			if public {
				finalDirectives = append(finalDirectives, string(DirPublic))
			} else {
				finalDirectives = append(finalDirectives, string(DirPrivate))
			}
			finalDirectives = append(finalDirectives, fmt.Sprintf("%s=%d", DirMaxAge, maxage))
			for _, i := range directives {
				finalDirectives = append(finalDirectives, string(i))
			}

			w2 := &cacheControlWriter{
				ResponseWriter: c.Res(),
				valCC:          strings.Join(finalDirectives, ", "),
				valETag:        etag,
				wroteHeader:    false,
			}

			c = governor.NewContext(w2, c.Req(), c.Log())

			next.ServeHTTPCtx(c)
		})
	}
}

// Control creates a middleware function to cache the response
func Control(log klog.Logger, public bool, directives Directives, maxage int64, etagfunc func(governor.Context) (string, error)) governor.Middleware {
	return governor.MiddlewareFromCtx(log, ControlCtx(public, directives, maxage, etagfunc))
}

// ControlNoStoreCtx creates a middleware function to deny caching responses
func ControlNoStoreCtx(next governor.RouteHandler) governor.RouteHandler {
	return governor.RouteHandlerFunc(func(c governor.Context) {
		c.SetHeader(headerCacheControl, string(DirNoStore))
		next.ServeHTTPCtx(c)
	})
}

// ControlNoStore creates a middleware function to deny caching responses
func ControlNoStore(log klog.Logger) governor.Middleware {
	return governor.MiddlewareFromCtx(log, ControlNoStoreCtx)
}
