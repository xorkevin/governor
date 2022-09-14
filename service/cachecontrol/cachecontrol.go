package cachecontrol

import (
	"fmt"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/klog"
)

const (
	ccHeader          = "Cache-Control"
	ifNoneMatchHeader = "If-None-Match"
	etagHeader        = "ETag"
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

			if val := c.Header(ifNoneMatchHeader); etag != "" && val == etag {
				c.WriteStatus(http.StatusNotModified)
				return
			}

			if public {
				c.SetHeader(ccHeader, string(DirPublic))
			} else {
				c.SetHeader(ccHeader, string(DirPrivate))
			}

			for _, i := range directives {
				c.AddHeader(ccHeader, string(i))
			}

			c.AddHeader(ccHeader, fmt.Sprintf("%s=%d", DirMaxAge, maxage))

			if etag != "" {
				c.SetHeader(etagHeader, etag)
			}

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
		c.SetHeader(ccHeader, string(DirNoStore))
		next.ServeHTTPCtx(c)
	})
}

// ControlNoStore creates a middleware function to deny caching responses
func ControlNoStore(log klog.Logger) governor.Middleware {
	return governor.MiddlewareFromCtx(log, ControlNoStoreCtx)
}
