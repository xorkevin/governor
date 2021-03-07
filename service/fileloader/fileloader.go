package fileloader

import (
	"io"
	"mime"
	"net/http"

	"xorkevin.dev/governor"
)

type (
	// ErrInvalidFile is returned when the uploaded file is invalid
	ErrInvalidFile struct{}
	// ErrUnsupportedMIME is returned when the uploaded file type is unsupported
	ErrUnsupportedMIME struct{}
)

func (e ErrInvalidFile) Error() string {
	return "Invalid file"
}

func (e ErrUnsupportedMIME) Error() string {
	return "Invalid file mime"
}

// LoadOpenFile returns an open file from a Context
func LoadOpenFile(l governor.Logger, c governor.Context, formField string, mimeTypes map[string]struct{}) (io.ReadSeekCloser, string, int64, error) {
	file, header, err := c.FormFile(formField)
	if err != nil {
		return nil, "", 0, governor.NewError(governor.ErrOptKind(ErrInvalidFile{}), governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Invalid file format",
		}), governor.ErrOptInner(err))
	}
	shouldClose := true
	defer func() {
		if shouldClose {
			if err := file.Close(); err != nil {
				l.Error("Failed to close open file on request", map[string]string{
					"actiontype": "closefile",
					"error":      err.Error(),
				})
			}
		}
	}()

	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		return nil, "", 0, governor.NewError(governor.ErrOptKind(ErrInvalidFile{}), governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "File does not have a media type",
		}), governor.ErrOptInner(err))
	}
	if len(mimeTypes) > 0 {
		if _, ok := mimeTypes[mediaType]; !ok {
			return nil, "", 0, governor.NewError(governor.ErrOptKind(ErrUnsupportedMIME{}), governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusUnsupportedMediaType,
				Message: mediaType + " is unsupported",
			}))
		}
	}
	shouldClose = false
	return file, mediaType, header.Size, nil
}
