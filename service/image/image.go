package image

import (
	"bytes"
	"github.com/hackform/governor"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	goimg "image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"mime"
	"net/http"
	"strconv"
)

const (
	moduleID = "image"
)

const (
	// MediaTypeJpeg is the mime type for jpeg images
	MediaTypeJpeg = "image/jpeg"
	// MediaTypePng is the mime type for png images
	MediaTypePng = "image/png"
	// MediaTypeGif is the mime type for gif images
	MediaTypeGif = "image/gif"
)

type (
	// Image is a service for managing image uploads
	Image interface {
		LoadJpeg(formField string, sizeLimit int64, quality int, contextField, contextFieldB64 string) echo.MiddlewareFunc
	}

	imageService struct {
		log *logrus.Logger
	}
)

// New returns a new image service
func New(conf governor.Config, l *logrus.Logger) Image {
	l.Info("initialized image service")

	return &imageService{
		log: l,
	}
}

const (
	moduleIDLoad = moduleID + ".Load"
)

// LoadJpeg reads an image from a form and places it into context as a jpeg
// sizeLimit is measured in bytes
func (im *imageService) LoadJpeg(formField string, sizeLimit int64, quality int, contextField, contextFieldB64 string) echo.MiddlewareFunc {
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
			switch mediaType {
			case MediaTypeJpeg, MediaTypePng, MediaTypeGif:
			default:
				return governor.NewErrorUser(moduleIDLoad, mediaType+" is unsupported", 0, http.StatusUnsupportedMediaType)
			}

			src, err := file.Open()
			if err != nil {
				return governor.NewError(moduleIDLoad, err.Error(), 0, http.StatusInternalServerError)
			}
			defer func(closer io.Closer) {
				err := closer.Close()
				if err != nil {
					gerr := governor.NewError(moduleIDLoad, err.Error(), 0, http.StatusInternalServerError)
					im.log.WithFields(logrus.Fields{
						"origin": gerr.Origin(),
						"source": gerr.Source(),
						"code":   gerr.Code(),
					}).Error(gerr.Message())
				}
			}(src)

			var img goimg.Image
			switch mediaType {
			case MediaTypeJpeg:
				if i, err := jpeg.Decode(src); err == nil {
					img = i
				} else {
					return governor.NewErrorUser(moduleIDLoad, err.Error(), 0, http.StatusBadRequest)
				}
			case MediaTypePng:
				if i, err := png.Decode(src); err == nil {
					img = i
				} else {
					return governor.NewErrorUser(moduleIDLoad, err.Error(), 0, http.StatusBadRequest)
				}
			case MediaTypeGif:
				if i, err := gif.Decode(src); err == nil {
					img = i
				} else {
					return governor.NewErrorUser(moduleIDLoad, err.Error(), 0, http.StatusBadRequest)
				}
			}

			b := &bytes.Buffer{}
			opt := jpeg.Options{
				Quality: quality,
			}
			if err := jpeg.Encode(b, img, &opt); err != nil {
				return governor.NewError(moduleIDLoad, err.Error(), 0, http.StatusInternalServerError)
			}

			c.Set(contextField, b)

			return next(c)
		}
	}
}

func humanReadableSize(size int64) string {
	switch {
	case size > 2000000:
		return strconv.FormatInt(size/1000000, 10) + "MB"
	case size > 2000:
		return strconv.FormatInt(size/1000, 10) + "KB"
	default:
		return strconv.FormatInt(size, 10) + "B"
	}
}
