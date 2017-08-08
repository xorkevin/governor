package governor

import (
	"encoding/json"
	"fmt"
	"github.com/labstack/echo"
	"mime"
	"net/http"
)

const (
	moduleIDBind = "govbind"
)

type (
	govBinder struct{}
)

func (gb *govBinder) Bind(i interface{}, c echo.Context) error {
	req := c.Request()
	if req.ContentLength == 0 {
		return NewErrorUser(moduleIDBind, "empty request body", 0, http.StatusBadRequest)
	}
	mediaType, _, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if err != nil {
		return NewErrorUser(moduleIDBind, err.Error(), 0, http.StatusBadRequest)
	}
	switch mediaType {
	case "application/json":
		if err := json.NewDecoder(req.Body).Decode(i); err != nil {
			if ute, ok := err.(*json.UnmarshalTypeError); ok {
				return NewErrorUser(moduleIDBind, fmt.Sprintf("Unmarshal type error: expected=%v, got=%v, offset=%v", ute.Type, ute.Value, ute.Offset), 0, http.StatusBadRequest)
			} else if se, ok := err.(*json.SyntaxError); ok {
				return NewErrorUser(moduleIDBind, fmt.Sprintf("Syntax error: offset=%v, error=%v", se.Offset, se.Error()), 0, http.StatusBadRequest)
			} else {
				return NewErrorUser(moduleIDBind, err.Error(), 0, http.StatusBadRequest)
			}
		}
	default:
		return NewErrorUser(moduleIDBind, "unsupported type", 0, http.StatusUnsupportedMediaType)
	}
	return nil
}

func requestBinder() echo.Binder {
	return &govBinder{}
}
