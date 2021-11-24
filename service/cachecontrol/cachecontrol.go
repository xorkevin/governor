package cachecontrol

import (
	"fmt"
	"net/http"

	"xorkevin.dev/governor"
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

// Control creates a middleware function to cache the response
func Control(l governor.Logger, public bool, directives Directives, maxage int64, etagfunc func(governor.Context) (string, error)) governor.Middleware {
	if maxage < 0 {
		panic("maxage cannot be negative")
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := governor.NewContext(w, r, l)

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

			next.ServeHTTP(c.R())
		})
	}
}

// ControlNoStore creates a middleware function to deny caching responses
func ControlNoStore(l governor.Logger) governor.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := governor.NewContext(w, r, l)
			c.SetHeader(ccHeader, string(DirNoStore))
			next.ServeHTTP(c.R())
		})
	}
}
