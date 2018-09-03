package fileloader

import (
	"github.com/hackform/governor"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"io"
	"mime"
	"net/http"
)

const (
	moduleID            = "fileloader"
	defaultContextField = "file"
	defaultSizeField    = "filesize"
)

type (
	// FileLoader is a service for managing file uploads
	FileLoader interface {
		Load(formField string, opt Options) echo.MiddlewareFunc
	}

	fileloaderService struct {
		log *logrus.Logger
	}

	// Options represent the options to load the file
	Options struct {
		ContextField string
		SizeField    string
		MimeType     []string
	}
)

// New returns a new fileloader service
func New(conf governor.Config, l *logrus.Logger) FileLoader {
	l.Info("initialized fileloader service")

	return &fileloaderService{
		log: l,
	}
}

const (
	moduleIDLoad = moduleID + ".Load"
)

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
				return governor.NewErrorUser(moduleIDLoad, err.Error(), 0, http.StatusBadRequest)
			}

			mediaType, _, err := mime.ParseMediaType(file.Header.Get("Content-Type"))
			if err != nil {
				return governor.NewErrorUser(moduleIDLoad, err.Error(), 0, http.StatusBadRequest)
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
					return governor.NewErrorUser(moduleIDLoad, mediaType+" is unsupported", 0, http.StatusUnsupportedMediaType)
				}
			}

			src, err := file.Open()
			if err != nil {
				return governor.NewError(moduleIDLoad, err.Error(), 0, http.StatusInternalServerError)
			}
			defer func(closer io.Closer) {
				err := closer.Close()
				if err != nil {
					gerr := governor.NewError(moduleIDLoad, err.Error(), 0, http.StatusInternalServerError)
					f.log.WithFields(logrus.Fields{
						"origin": gerr.Origin(),
						"source": gerr.Source(),
						"code":   gerr.Code(),
					}).Error(gerr.Message())
				}
			}(src)

			c.Set(opt.ContextField, src)
			c.Set(opt.SizeField, file.Size)

			return next(c)
		}
	}
}
