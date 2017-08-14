package cachecontrol

import (
	"github.com/labstack/echo"
	"strconv"
)

type (
	// CacheControl is a service for managing http cache-control headers
	CacheControl interface {
		Control(public, revalidate bool, maxage int) echo.MiddlewareFunc
		NoStore() echo.MiddlewareFunc
	}

	cacheControl struct {
	}
)

// New creates a new cache control service
func New() CacheControl {
	return &cacheControl{}
}

const (
	ccHeader  = "Cache-Control"
	ccPublic  = "public"
	ccPrivate = "private"
	ccNoCache = "no-cache"
	ccNoStore = "no-store"
	ccMaxAge  = "max-age"
)

// Control creates a middleware function to cache the response
func (c *cacheControl) Control(public, revalidate bool, maxage int) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if err := next(c); err != nil {
				return err
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

			if maxage >= 0 {
				resheader.Add(ccHeader, ccMaxAge+"="+strconv.Itoa(maxage))
			}

			return nil
		}
	}
}

// NoStore creates a middleware function to deny caching responses
func (c *cacheControl) NoStore() echo.MiddlewareFunc {
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
