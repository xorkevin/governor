package fileloader

import (
	"fmt"
	"io"
	"mime"
	"net/http"

	"xorkevin.dev/governor"
)

// LoadOpenFile returns an open file from a Context
func LoadOpenFile(l governor.Logger, c governor.Context, formField string, mimeTypes map[string]struct{}) (io.ReadSeekCloser, string, int64, error) {
	file, header, err := c.FormFile(formField)
	if err != nil {
		return nil, "", 0, governor.NewErrorUser("Invalid file format", http.StatusBadRequest, err)
	}
	shouldClose := true
	defer func() {
		if shouldClose {
			if err := file.Close(); err != nil {
				l.Error("failed to close open file on request", map[string]string{
					"actiontype": "closefile",
					"error":      err.Error(),
				})
			}
		}
	}()

	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		return nil, "", 0, governor.NewErrorUser("File does not have a media type", http.StatusBadRequest, err)
	}
	if len(mimeTypes) > 0 {
		if _, ok := mimeTypes[mediaType]; !ok {
			return nil, "", 0, governor.NewErrorUser(fmt.Sprintf("%s is unsupported", mediaType), http.StatusUnsupportedMediaType, nil)
		}
	}
	shouldClose = false
	return file, mediaType, header.Size, nil
}
