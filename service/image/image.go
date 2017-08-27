package image

import (
	"bytes"
	"encoding/base64"
	"github.com/hackform/governor"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"golang.org/x/image/draw"
	goimg "image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"mime"
	"net/http"
)

const (
	moduleID = "image"

	// MediaTypeJpeg is the mime type for jpeg images
	MediaTypeJpeg = "image/jpeg"
	// MediaTypePng is the mime type for png images
	MediaTypePng = "image/png"
	// MediaTypeGif is the mime type for gif images
	MediaTypeGif = "image/gif"

	dataURIPrefixJpeg = "data:image/jpeg;base64,"

	thumbnailWidth  = 42
	thumbnailHeight = 24
)

type (
	// Image is a service for managing image uploads
	Image interface {
		LoadJpeg(formField string, width, height int, crop bool, quality int, contextField, contextFieldThumb string) echo.MiddlewareFunc
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
func (im *imageService) LoadJpeg(formField string, width, height int, crop bool, quality int, contextField, contextFieldThumb string) echo.MiddlewareFunc {
	if formField == "" {
		panic("formField cannot be an empty string")
	}
	if width < 1 || height < 1 {
		panic("width and height must be a positive integer")
	}
	if quality < 1 || quality > 100 {
		panic("quality must be between 1 and 100")
	}
	if contextField == "" {
		panic("contextField cannot be an empty string")
	}
	if contextFieldThumb == "" {
		panic("contextFieldB64 cannot be an empty string")
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

			if crop {
				img = resizeImgCrop(img, width, height)
			} else {
				img = resizeImg(img, width, height)
			}

			thumb := resizeImg(img, thumbnailWidth, thumbnailHeight)

			b := &bytes.Buffer{}
			b2 := &bytes.Buffer{}
			opt := jpeg.Options{
				Quality: quality,
			}
			if err := jpeg.Encode(b, img, &opt); err != nil {
				return governor.NewError(moduleIDLoad, err.Error(), 0, http.StatusInternalServerError)
			}
			if err := jpeg.Encode(b2, thumb, &opt); err != nil {
				return governor.NewError(moduleIDLoad, err.Error(), 0, http.StatusInternalServerError)
			}
			b64 := base64.StdEncoding.EncodeToString(b2.Bytes())

			c.Set(contextField, b)
			c.Set(contextFieldThumb, dataURIPrefixJpeg+b64)

			return next(c)
		}
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func resizeImg(img goimg.Image, width, height int) goimg.Image {
	s := img.Bounds().Size()
	if s.X < width && s.Y < height {
		return img
	}

	var targetWidth, targetHeight int
	targetRatio := float32(width) / float32(height)
	origRatio := float32(s.X) / float32(s.Y)
	if origRatio < targetRatio {
		targetHeight = height
		targetWidth = minInt(int(float32(targetHeight)*origRatio), width)
	} else {
		targetWidth = width
		targetHeight = minInt(int(float32(targetWidth)/origRatio), height)
	}

	target := goimg.NewNRGBA(goimg.Rect(0, 0, targetWidth, targetHeight))
	draw.Draw(target, target.Bounds(), goimg.White, goimg.ZP, draw.Src)
	draw.ApproxBiLinear.Scale(target, target.Bounds(), img, img.Bounds(), draw.Src, nil)

	return target
}

func resizeImgCrop(img goimg.Image, width, height int) goimg.Image {
	s := img.Bounds().Size()

	var targetWidth, targetHeight, imgWidth, imgHeight int
	targetRatio := float32(width) / float32(height)
	origRatio := float32(s.X) / float32(s.Y)

	var imgBounds goimg.Rectangle

	if origRatio < targetRatio {
		imgWidth = s.X
		imgHeight = minInt(int(float32(imgWidth)/targetRatio), s.Y)
		k := (s.Y - imgHeight) / 2
		imgBounds = goimg.Rect(0, k, imgWidth, k+imgHeight)
	} else {
		imgHeight = s.Y
		imgWidth = minInt(int(float32(imgHeight)*targetRatio), s.X)
		k := (s.X - imgWidth) / 2
		imgBounds = goimg.Rect(k, 0, k+imgWidth, imgHeight)
	}

	if s.X > width && s.Y > height {
		targetWidth = width
		targetHeight = height
	} else {
		targetWidth = imgWidth
		targetHeight = imgHeight
	}

	target := goimg.NewNRGBA(goimg.Rect(0, 0, targetWidth, targetHeight))
	draw.Draw(target, target.Bounds(), goimg.White, goimg.ZP, draw.Src)
	draw.ApproxBiLinear.Scale(target, target.Bounds(), img, imgBounds, draw.Src, nil)

	return target
}
