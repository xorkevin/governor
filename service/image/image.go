package image

import (
	"bytes"
	"encoding/base64"
	"github.com/labstack/echo/v4"
	"golang.org/x/image/draw"
	goimg "image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/fileloader"
)

type (
	Size struct {
		W, H int
	}

	// Image is an open image file
	Image interface {
		goimg.Image
		Duplicate() Image
		Size() Size
		Draw(img Image, x, y int, over bool)
		Resize(width, height int)
		ResizeFit(width, height int)
		ResizeLimit(width, height int)
		Crop(x, y, w, h int)
		ResizeFill(width, height int)
		ToJpeg(quality int) (*bytes.Buffer, error)
		ToPng(level PngCompressionOpt) (*bytes.Buffer, error)
		ToBase64(quality int) (string, error)
	}

	imageData struct {
		img draw.Image
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

func FromImage(img goimg.Image) Image {
	bounds := img.Bounds()
	target := goimg.NewNRGBA64(bounds)
	draw.Draw(target, target.Bounds(), img, bounds.Min, draw.Src)
	return &imageData{
		img: target,
	}
}

func FromJpeg(file io.Reader) (Image, error) {
	i, err := jpeg.Decode(file)
	if err != nil {
		return nil, governor.NewError("Invalid JPEG image", http.StatusBadRequest, err)
	}
	return FromImage(i), nil
}

func FromPng(file io.Reader) (Image, error) {
	i, err := png.Decode(file)
	if err != nil {
		return nil, governor.NewError("Invalid PNG image", http.StatusBadRequest, err)
	}
	return FromImage(i), nil
}

func FromGif(file io.Reader) (Image, error) {
	i, err := gif.Decode(file)
	if err != nil {
		return nil, governor.NewError("Invalid GIF image", http.StatusBadRequest, err)
	}
	return FromImage(i), nil
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
		}
	}()
	switch mediaType {
	case MediaTypeJpeg:
		return FromJpeg(file)
	case MediaTypePng:
		return FromPng(file)
	case MediaTypeGif:
		return FromGif(file)
	default:
		return nil, governor.NewErrorUser("Invalid file type", http.StatusBadRequest, err)
	}
}

func (i imageData) ColorModel() color.Model {
	return i.img.ColorModel()
}

func (i imageData) Bounds() goimg.Rectangle {
	return i.img.Bounds()
}

func (i imageData) At(x, y int) color.Color {
	return i.img.At(x, y)
}

func (i imageData) Duplicate() Image {
	return FromImage(i.img)
}

func (i imageData) Size() Size {
	k := i.img.Bounds().Size()
	return Size{
		W: k.X,
		H: k.Y,
	}
}

func (i *imageData) Draw(img Image, x, y int, over bool) {
	source := img.Bounds()
	op := draw.Src
	if over {
		op = draw.Over
	}
	draw.Draw(i.img, source.Sub(source.Min).Add(i.img.Bounds().Min).Add(goimg.Pt(x, y)), img, source.Min, op)
}

func (i *imageData) Resize(width, height int) {
	target := goimg.NewNRGBA64(goimg.Rect(0, 0, width, height))
	draw.Draw(target, target.Bounds(), goimg.Transparent, goimg.ZP, draw.Src)
	draw.ApproxBiLinear.Scale(target, target.Bounds(), i.img, i.img.Bounds(), draw.Src, nil)
	i.img = target
}

func dimensionsFit(fromWidth, fromHeight, toWidth, toHeight int) (int, int) {
	// fromRatio < toRatio
	if fromWidth*toHeight < toWidth*fromHeight {
		// height is fit
		return fromWidth * toHeight / fromHeight, toHeight
	} else {
		// width is fit
		return toWidth, fromHeight * toWidth / fromWidth
	}
}

func (i *imageData) ResizeFit(width, height int) {
	s := i.img.Bounds().Size()
	targetWidth, targetHeight := dimensionsFit(s.X, s.Y, width, height)
	i.Resize(targetWidth, targetHeight)
}

func (i *imageData) ResizeLimit(width, height int) {
	s := i.img.Bounds().Size()
	if s.X < width && s.Y < height {
		return
	}
	i.ResizeFit(width, height)
}

func (i *imageData) Crop(x, y, w, h int) {
	target := goimg.NewNRGBA64(goimg.Rect(0, 0, w, h))
	draw.Draw(target, target.Bounds(), i.img, goimg.Pt(x, y), draw.Src)
	i.img = target
}

func maxInt(a, b int) int {
	if a < b {
		return b
	}
	return a
}

func dimensionsFill(fromWidth, fromHeight, toWidth, toHeight int) (int, int, int, int) {
	// fromRatio < toRatio
	if fromWidth*toHeight < toWidth*fromHeight {
		// width is fit
		height := toHeight * fromWidth / toWidth
		return fromWidth, height, 0, maxInt((fromHeight-height)/2, 0)
	} else {
		// height is fit
		width := toWidth * fromHeight / toHeight
		return width, fromHeight, maxInt((fromWidth-width)/2, 0), 0
	}
}

func (i *imageData) ResizeFill(width, height int) {
	s := i.img.Bounds().Size()
	targetWidth, targetHeight, offsetX, offsetY := dimensionsFill(s.X, s.Y, width, height)
	target := goimg.NewNRGBA64(goimg.Rect(0, 0, width, height))
	draw.Draw(target, target.Bounds(), goimg.Transparent, goimg.ZP, draw.Src)
	draw.ApproxBiLinear.Scale(target, target.Bounds(), i.img, goimg.Rect(offsetX, offsetY, offsetX+targetWidth, offsetY+targetHeight), draw.Src, nil)
	i.img = target
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

type PngCompressionOpt int

const (
	PngDefault PngCompressionOpt = iota
	PngNone
	PngFast
	PngBest
)

func compressionOptTranslate(level PngCompressionOpt) png.CompressionLevel {
	switch level {
	case PngDefault:
		return png.DefaultCompression
	case PngNone:
		return png.NoCompression
	case PngFast:
		return png.BestSpeed
	case PngBest:
		return png.BestCompression
	default:
		return png.DefaultCompression
	}
}

func (i *imageData) ToPng(level PngCompressionOpt) (*bytes.Buffer, error) {
	b := &bytes.Buffer{}
	encoder := png.Encoder{
		CompressionLevel: compressionOptTranslate(level),
	}
	if err := encoder.Encode(b, i.img); err != nil {
		return nil, governor.NewError("Failed to encode PNG image", http.StatusInternalServerError, err)
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
