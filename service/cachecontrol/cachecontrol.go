package cachecontrol

import (
	"fmt"
	"net/http"
	"xorkevin.dev/governor"
)

const (
	ccHeader = "Cache-Control"
)

func etagToValue(etag string) string {
	return fmt.Sprintf(`W/"%s"`, etag)
}

// Control creates a middleware function to cache the response
func Control(l governor.Logger, public, revalidate bool, maxage int64, etagfunc func(governor.Context) (string, error)) governor.Middleware {
	if maxage < 0 {
		panic("maxage cannot be negative")
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := governor.NewContext(w, r, l)

			etag := ""
			if etagfunc != nil {
				if tag, err := etagfunc(c); err != nil {
					etag = etagToValue(tag)
				} else {
					c.WriteError(err)
					return
				}
			}

			if val := c.Header("If-None-Match"); etag != "" && val == etag {
				c.WriteStatus(http.StatusNotModified)
				return
			}

			if public {
				c.SetHeader(ccHeader, "public")
			} else {
				c.SetHeader(ccHeader, "private")
			}

			if revalidate {
				c.AddHeader(ccHeader, "no-cache")
			}

			c.AddHeader(ccHeader, fmt.Sprintf("maxage=%d", maxage))

			if etag != "" {
				c.SetHeader("ETag", etag)
			}

			next.ServeHTTP(c.R())
		})
	}
}

// NoStore creates a middleware function to deny caching responses
func NoStore(l governor.Logger) governor.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := governor.NewContext(w, r, l)
			c.SetHeader(ccHeader, "no-store")
			next.ServeHTTP(c.R())
		})
	}
}
