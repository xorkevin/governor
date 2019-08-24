package image

import (
	"bytes"
	"encoding/base64"
	"github.com/labstack/echo"
	"golang.org/x/image/draw"
	goimg "image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/fileloader"
)

const (
	defaultThumbnailWidth  = 24
	defaultThumbnailHeight = 24
	defaultQuality         = 85
	defaultThumbQuality    = 50
)

type (
	// Image is an open image file
	Image interface {
		Duplicate() Image
		Resize(width, height int)
		ResizeFit(width, height int)
		ResizeLimit(width, height int)
		Crop(bounds goimg.Rectangle)
		ResizeFill(width, height int)
		ToJpeg(quality int) (*bytes.Buffer, error)
		ToBase64(quality int) (string, error)
	}

	imageData struct {
		img goimg.Image
	}

	// Options represent the image options of the loaded image
	Options struct {
		Width        int
		Height       int
		ThumbWidth   int
		ThumbHeight  int
		Quality      int
		ThumbQuality int
		Fill         bool
	}
)

// LoadJpeg reads an image from a form and encodes it as a jpeg
func LoadJpeg(c echo.Context, formField string, opt Options) (*bytes.Buffer, string, error) {
	if opt.Width < 1 || opt.Height < 1 {
		opt.Width = defaultThumbnailWidth
		opt.Height = defaultThumbnailHeight
	}
	if opt.ThumbWidth < 1 || opt.ThumbHeight < 1 {
		opt.ThumbWidth = defaultThumbnailWidth
		opt.ThumbHeight = defaultThumbnailHeight
	}
	if opt.Quality < 1 || opt.Quality > 100 {
		opt.Quality = defaultQuality
	}
	if opt.ThumbQuality < 1 || opt.ThumbQuality > 100 {
		opt.ThumbQuality = defaultThumbQuality
	}

	img, err := LoadImage(c, formField)
	if err != nil {
		return nil, "", err
	}
	if opt.Fill {
		img.ResizeFill(opt.Width, opt.Height)
	} else {
		img.ResizeLimit(opt.Width, opt.Height)
	}
	thumb := img.Duplicate()
	thumb.ResizeLimit(opt.ThumbWidth, opt.ThumbHeight)

	b, err := img.ToJpeg(opt.Quality)
	if err != nil {
		return nil, "", governor.NewError("Failed to encode image as JPEG", http.StatusInternalServerError, err)
	}
	b2, err := thumb.ToBase64(opt.ThumbQuality)
	if err != nil {
		return nil, "", governor.NewError("Failed to encode thumbnail as JPEG", http.StatusInternalServerError, err)
	}
	return b, b2, nil
}

const (
	// MediaTypeJpeg is the mime type for jpeg images
	MediaTypeJpeg = "image/jpeg"
	// MediaTypePng is the mime type for png images
	MediaTypePng = "image/png"
	// MediaTypeGif is the mime type for gif images
	MediaTypeGif = "image/gif"
)

func LoadImage(c echo.Context, formField string) (Image, error) {
	file, mediaType, _, err := fileloader.LoadOpenFile(c, formField, []string{MediaTypePng, MediaTypeJpeg, MediaTypeGif})
	if err != nil {
		return nil, governor.NewErrorUser("Invalid image file", http.StatusBadRequest, err)
	}
	defer func() {
		err := file.Close()
		if err != nil {
			//im.log.Error("image: fail close image file", map[string]string{
			//	"err": err.Error(),
			//})
		}
	}()
	var img goimg.Image
	switch mediaType {
	case MediaTypeJpeg:
		if i, err := jpeg.Decode(file); err != nil {
			return nil, governor.NewErrorUser("Invalid JPEG image", http.StatusBadRequest, err)
		} else {
			img = i
		}
	case MediaTypePng:
		if i, err := png.Decode(file); err != nil {
			return nil, governor.NewErrorUser("Invalid PNG image", http.StatusBadRequest, err)
		} else {
			img = i
		}
	case MediaTypeGif:
		if i, err := gif.Decode(file); err != nil {
			return nil, governor.NewErrorUser("Invalid GIF image", http.StatusBadRequest, err)
		} else {
			img = i
		}
	}
	return &imageData{
		img: img,
	}, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (i imageData) Duplicate() Image {
	bounds := i.img.Bounds()
	target := goimg.NewNRGBA(bounds)
	draw.Draw(target, target.Bounds(), i.img, bounds.Min, draw.Src)
	return &imageData{
		img: target,
	}
}

func (i *imageData) Resize(width, height int) {
	target := goimg.NewNRGBA(goimg.Rect(0, 0, width, height))
	draw.Draw(target, target.Bounds(), goimg.Transparent, goimg.ZP, draw.Src)
	draw.ApproxBiLinear.Scale(target, target.Bounds(), i.img, i.img.Bounds(), draw.Src, nil)
	i.img = target
}

func (i *imageData) ResizeFit(width, height int) {
	s := i.img.Bounds().Size()
	targetRatio := float64(width) / float64(height)
	origRatio := float64(s.X) / float64(s.Y)
	var targetWidth, targetHeight int
	if origRatio < targetRatio {
		targetHeight = height
		targetWidth = minInt(int(float64(targetHeight)*origRatio), width)
	} else {
		targetWidth = width
		targetHeight = minInt(int(float64(targetWidth)/origRatio), height)
	}
	i.Resize(targetWidth, targetHeight)
}

func (i *imageData) ResizeLimit(width, height int) {
	s := i.img.Bounds().Size()
	if s.X < width && s.Y < height {
		return
	}
	i.ResizeFit(width, height)
}

func (i *imageData) Crop(bounds goimg.Rectangle) {
	size := bounds.Size()
	target := goimg.NewNRGBA(goimg.Rect(0, 0, size.X, size.Y))
	draw.Draw(target, target.Bounds(), goimg.Transparent, goimg.ZP, draw.Src)
	draw.Draw(target, target.Bounds(), i.img, bounds.Min, draw.Src)
	i.img = target
}

func (i *imageData) ResizeFill(width, height int) {
	s := i.img.Bounds().Size()
	targetRatio := float64(width) / float64(height)
	origRatio := float64(s.X) / float64(s.Y)
	var imgBounds goimg.Rectangle

	if origRatio < targetRatio {
		imgHeight := minInt(int(float64(s.X)/targetRatio), s.Y)
		k := (s.Y - imgHeight) / 2
		imgBounds = goimg.Rect(0, k, s.X, k+imgHeight)
	} else {
		imgWidth := minInt(int(float64(s.Y)*targetRatio), s.X)
		k := (s.X - imgWidth) / 2
		imgBounds = goimg.Rect(k, 0, k+imgWidth, s.Y)
	}

	i.Crop(imgBounds)
	i.ResizeFit(width, height)
}

func (i *imageData) ToJpeg(quality int) (*bytes.Buffer, error) {
	b := &bytes.Buffer{}
	j := jpeg.Options{
		Quality: quality,
	}
	if err := jpeg.Encode(b, i.img, &j); err != nil {
		return nil, governor.NewError("Failed to encode JPEG image", http.StatusInternalServerError, err)
	}
	return b, nil
}

const (
	dataURIPrefixJpeg = "data:image/jpeg;base64,"
)

func (i *imageData) ToBase64(quality int) (string, error) {
	b, err := i.ToJpeg(quality)
	if err != nil {
		return "", err
	}
	return dataURIPrefixJpeg + base64.RawStdEncoding.EncodeToString(b.Bytes()), nil
}
