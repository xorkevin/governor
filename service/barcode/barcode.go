package barcode

import (
	"bytes"
	bar "github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"
	"github.com/hackform/governor"
	"image/png"
	"net/http"
)

const (
	barQRSize = 256
)

const (
	// TransportQRCode is a type constant for QRCode
	TransportQRCode = iota
)

type (
	// Generator is a service that encodes data into barcodes
	Generator interface {
		GenerateBarcode(transport int, data string) (*bytes.Buffer, error)
	}

	barcodeService struct {
		encoder *png.Encoder
	}
)

// New returns a new barcode service
func New(conf governor.Config, l governor.Logger) Generator {
	l.Info("initialize barcode service", nil)

	return &barcodeService{
		encoder: &png.Encoder{
			CompressionLevel: png.BestCompression,
		},
	}
}

// GenerateBarcode encodes data as a barcode image represented by a bytes Buffer
func (b *barcodeService) GenerateBarcode(transport int, data string) (*bytes.Buffer, error) {
	switch transport {
	case TransportQRCode:
		qrCode, err := qr.Encode(data, qr.H, qr.Unicode)
		if err != nil {
			return nil, governor.NewError("Fail to encode qr data", http.StatusInternalServerError, err)
		}

		qrCode, err = bar.Scale(qrCode, barQRSize, barQRSize)
		if err != nil {
			return nil, governor.NewError("Fail to scale qr code", http.StatusInternalServerError, err)
		}

		buf := &bytes.Buffer{}
		if err := b.encoder.Encode(buf, qrCode); err != nil {
			return nil, governor.NewError("Fail to encode qr code to bytes", http.StatusInternalServerError, err)
		}

		return buf, nil
	}

	return nil, governor.NewError("Invalid transport", http.StatusInternalServerError, nil)
}
