package fileloader

import (
	"io"
	"mime"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	// ErrorInvalidFile is returned when the uploaded file is invalid
	ErrorInvalidFile struct{}
	// ErrorUnsupportedMIME is returned when the uploaded file type is unsupported
	ErrorUnsupportedMIME struct{}
)

func (e ErrorInvalidFile) Error() string {
	return "Invalid file"
}

func (e ErrorUnsupportedMIME) Error() string {
	return "Invalid file mime"
}

// LoadOpenFile returns an open file from a Context
func LoadOpenFile(c *governor.Context, formField string, mimeTypes map[string]struct{}) (io.ReadSeekCloser, string, int64, error) {
	file, header, err := c.FormFile(formField)
	if err != nil {
		return nil, "", 0, governor.ErrWithRes(kerrors.WithKind(err, ErrorInvalidFile{}, "Invalid file format"), http.StatusBadRequest, "", "Invalid file format")
	}
	l := klog.NewLevelLogger(c.Log())
	shouldClose := true
	defer func() {
		if shouldClose {
			if err := file.Close(); err != nil {
				l.Err(c.Ctx(), kerrors.WithMsg(err, "Failed to close open file on request"), nil)
			}
		}
	}()

	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		return nil, "", 0, governor.ErrWithRes(kerrors.WithKind(err, ErrorInvalidFile{}, "No media type"), http.StatusBadRequest, "", "File does not have a media type")
	}
	if len(mimeTypes) > 0 {
		if _, ok := mimeTypes[mediaType]; !ok {
			return nil, "", 0, governor.ErrWithRes(kerrors.WithKind(nil, ErrorUnsupportedMIME{}, "Unsupported MIME type"), http.StatusUnsupportedMediaType, "", mediaType+" is unsupported")
		}
	}
	shouldClose = false
	return file, mediaType, header.Size, nil
}
