package barcode

import (
	bar "github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"
	"xorkevin.dev/governor/service/image"
	"xorkevin.dev/kerrors"
)

type (
	// QRECLevel represents qr error correction levels
	QRECLevel int
)

// QR error correction levels
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
		return nil, kerrors.WithMsg(err, "Failed to encode qr data")
	}
	size := qrCode.Bounds().Size()
	qrCode, err = bar.Scale(qrCode, size.X*scale, size.Y*scale)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to scale qr code")
	}
	return image.FromImage(qrCode), nil
}
