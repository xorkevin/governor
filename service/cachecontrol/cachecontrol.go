package cachecontrol

import (
	"fmt"
	"github.com/labstack/echo"
	"net/http"
)

const (
	ccHeader       = "Cache-Control"
	ccPublic       = "public"
	ccPrivate      = "private"
	ccNoCache      = "no-cache"
	ccNoStore      = "no-store"
	ccMaxAge       = "max-age"
	ccEtagH        = "ETag"
	ccEtagValue    = `W/"%s"`
	ccIfNoneMatchH = "If-None-Match"
)

func etagToValue(etag string) string {
	return fmt.Sprintf(ccEtagValue, etag)
}

// Control creates a middleware function to cache the response
func Control(public, revalidate bool, maxage int, etagfunc func(echo.Context) (string, error)) echo.MiddlewareFunc {
	if maxage < 0 {
		panic("maxage cannot be negative")
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			etag := ""

			if etagfunc != nil {
				if tag, err := etagfunc(c); err == nil {
					etag = etagToValue(tag)
				} else {
					return err
				}
			}

			if val := c.Request().Header.Get(ccIfNoneMatchH); etag != "" && val != "" {
				if val == etag {
					return c.NoContent(http.StatusNotModified)
				}
			}

			resheader := c.Response().Header()

			if public {
				resheader.Set(ccHeader, ccPublic)
			} else {
				resheader.Set(ccHeader, ccPrivate)
			}

			if revalidate {
				resheader.Add(ccHeader, ccNoCache)
			}

			resheader.Add(ccHeader, fmt.Sprintf("%s=%d", ccMaxAge, maxage))

			if etag != "" {
				resheader.Set(ccEtagH, etag)
			}

			return next(c)
		}
	}
}

// NoStore creates a middleware function to deny caching responses
func NoStore() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if err := next(c); err != nil {
				return err
			}

			resheader := c.Response().Header()
			resheader.Set(ccHeader, ccNoStore)

			return nil
		}
	}
}
