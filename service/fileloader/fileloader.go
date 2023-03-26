package fileloader

import (
	"errors"
	"io"
	"mime"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/kerrors"
)

var (
	// ErrorInvalidFile is returned when the uploaded file is invalid
	ErrorInvalidFile errorInvalidFile
	// ErrorUnsupportedMIME is returned when the uploaded file type is unsupported
	ErrorUnsupportedMIME errorUnsupportedMIME
)

type (
	errorInvalidFile     struct{}
	errorUnsupportedMIME struct{}
)

func (e errorInvalidFile) Error() string {
	return "Invalid file"
}

func (e errorUnsupportedMIME) Error() string {
	return "Invalid file mime"
}

// LoadOpenFile returns an open file from a Context
func LoadOpenFile(c *governor.Context, formField string, mimeTypes map[string]struct{}) (_ io.ReadSeekCloser, _ string, _ int64, retErr error) {
	file, header, err := c.FormFile(formField)
	if err != nil {
		return nil, "", 0, err
	}
	shouldClose := true
	defer func() {
		if shouldClose {
			if err := file.Close(); err != nil {
				retErr = errors.Join(retErr, kerrors.WithMsg(err, "Failed to close open file on request"))
			}
		}
	}()

	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		return nil, "", 0, governor.ErrWithRes(kerrors.WithKind(err, ErrorInvalidFile, "No media type"), http.StatusBadRequest, "", "File does not have a media type")
	}
	if len(mimeTypes) > 0 {
		if _, ok := mimeTypes[mediaType]; !ok {
			return nil, "", 0, governor.ErrWithRes(kerrors.WithKind(nil, ErrorUnsupportedMIME, "Unsupported MIME type"), http.StatusUnsupportedMediaType, "", mediaType+" is unsupported")
		}
	}
	shouldClose = false
	return file, mediaType, header.Size, nil
}
