package governor

import (
	"encoding/json"
	"github.com/labstack/echo"
	"mime"
	"net/http"
)

type (
	govBinder struct{}
)

func (gb *govBinder) Bind(i interface{}, c echo.Context) error {
	req := c.Request()
	if req.ContentLength == 0 {
		return NewErrorUser("Empty request body", http.StatusBadRequest, nil)
	}
	mediaType, _, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if err != nil {
		return NewErrorUser("Invalid mime type", http.StatusBadRequest, err)
	}
	switch mediaType {
	case "application/json":
		if err := json.NewDecoder(req.Body).Decode(i); err != nil {
			return NewErrorUser("Invalid JSON", http.StatusBadRequest, err)
		}
	default:
		return NewErrorUser("Unsupported media type", http.StatusUnsupportedMediaType, nil)
	}
	return nil
}

func requestBinder() echo.Binder {
	return &govBinder{}
}
