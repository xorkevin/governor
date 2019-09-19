package barcode

import (
	bar "github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"
	"net/http"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/image"
)

type QRECLevel int

const (
	QRECDefault QRECLevel = iota
	QRECLow
	QRECMedium
	QRECQuartile
	QRECHigh
)

func qrecTranslate(level QRECLevel) qr.ErrorCorrectionLevel {
	switch level {
	case QRECDefault:
		return qr.L
	case QRECLow:
		return qr.L
	case QRECMedium:
		return qr.M
	case QRECQuartile:
		return qr.Q
	case QRECHigh:
		return qr.H
	default:
		return qr.L
	}
}

// GenerateQR encodes data as a QR code image
func GenerateQR(data string, level QRECLevel, scale int) (image.Image, error) {
	qrCode, err := qr.Encode(data, qrecTranslate(level), qr.Unicode)
	if err != nil {
		return nil, governor.NewError("Failed to encode qr data", http.StatusInternalServerError, err)
	}
	size := qrCode.Bounds().Size()
	qrCode, err = bar.Scale(qrCode, size.X*scale, size.Y*scale)
	if err != nil {
		return nil, governor.NewError("Failed to scale qr code", http.StatusInternalServerError, err)
	}
	return image.FromImage(qrCode), nil
}
