package fileloader

import (
	"fmt"
	"github.com/labstack/echo"
	"io"
	"mime"
	"net/http"
	"xorkevin.dev/governor"
)

const (
	defaultContextField = "file"
	defaultSizeField    = "filesize"
)

type (
	// FileLoader is a service for managing file uploads
	FileLoader interface {
		Load(formField string, opt Options) echo.MiddlewareFunc
	}

	fileloaderService struct {
		log governor.Logger
	}

	// Options represent the options to load the file
	Options struct {
		ContextField string
		SizeField    string
		MimeType     []string
	}
)

// New returns a new fileloader service
func New(conf governor.Config, l governor.Logger) FileLoader {
	l.Info("initialize fileloader service", nil)

	return &fileloaderService{
		log: l,
	}
}

// Load reads in a file from a form and places it into context
func (f *fileloaderService) Load(formField string, opt Options) echo.MiddlewareFunc {
	if formField == "" {
		panic("formField cannot be an empty string")
	}
	if opt.ContextField == "" {
		opt.ContextField = defaultContextField
	}
	if opt.SizeField == "" {
		opt.SizeField = defaultSizeField
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			file, err := c.FormFile(formField)
			if err != nil {
				return governor.NewErrorUser("Invalid file format", http.StatusBadRequest, err)
			}

			mediaType, _, err := mime.ParseMediaType(file.Header.Get("Content-Type"))
			if err != nil {
				return governor.NewErrorUser("File does not have a media type", http.StatusBadRequest, err)
			}
			if opt.MimeType != nil && len(opt.MimeType) > 0 {
				found := false
				for _, i := range opt.MimeType {
					if i == mediaType {
						found = true
						break
					}
				}
				if !found {
					return governor.NewErrorUser(fmt.Sprintf("%s is unsupported", mediaType), http.StatusUnsupportedMediaType, nil)
				}
			}

			src, err := file.Open()
			if err != nil {
				return governor.NewErrorUser("Failed to open file", http.StatusInternalServerError, err)
			}
			defer func(closer io.Closer) {
				err := closer.Close()
				if err != nil {
					f.log.Error("fileloader: fail close file", map[string]string{
						"err": err.Error(),
					})
				}
			}(src)

			c.Set(opt.ContextField, src)
			c.Set(opt.SizeField, file.Size)

			return next(c)
		}
	}
}
