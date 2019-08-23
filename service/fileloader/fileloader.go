package fileloader

import (
	"fmt"
	"github.com/labstack/echo"
	"mime"
	"mime/multipart"
	"net/http"
	"xorkevin.dev/governor"
)

func LoadOpenFile(c echo.Context, formField string, mimeTypes []string) (multipart.File, string, int64, error) {
	file, err := c.FormFile(formField)
	if err != nil {
		return nil, "", 0, governor.NewErrorUser("Invalid file format", http.StatusBadRequest, err)
	}

	mediaType, _, err := mime.ParseMediaType(file.Header.Get("Content-Type"))
	if err != nil {
		return nil, "", 0, governor.NewErrorUser("File does not have a media type", http.StatusBadRequest, err)
	}
	if mimeTypes != nil && len(mimeTypes) > 0 {
		found := false
		for _, i := range mimeTypes {
			if i == mediaType {
				found = true
				break
			}
		}
		if !found {
			return nil, "", 0, governor.NewErrorUser(fmt.Sprintf("%s is unsupported", mediaType), http.StatusUnsupportedMediaType, nil)
		}
	}
	src, err := file.Open()
	if err != nil {
		return nil, "", 0, governor.NewErrorUser("Failed to open file", http.StatusInternalServerError, err)
	}
	return src, mediaType, file.Size, nil
}
