package governor

import (
	"bytes"
	"encoding/json"
	"github.com/labstack/echo/v4"
	"mime"
	"net/http"
)

type (
	Router interface {
	}

	govrouter struct {
		router *echo.Group
	}

	Middleware func(next http.Handler) http.Handler
)

func (s *Server) router(path string) Router {
	return &govrouter{
		router: s.i.Group(path),
	}
}

type (
	Context interface {
		WriteError(err error)
		WriteJSON(status int, body interface{})
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
