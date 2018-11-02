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
	moduleID  = "barcode"
	barQRSize = 256
)

const (
	// TransportQRCode is a type constant for QRCode
	TransportQRCode = iota
)

type (
	// Generator is a service that encodes data into barcodes
	Generator interface {
		GenerateBarcode(transport int, data string) (*bytes.Buffer, *governor.Error)
	}

	barcodeService struct {
		encoder *png.Encoder
	}
)

// New returns a new barcode service
func New(conf governor.Config, l governor.Logger) Generator {
	l.Info("initialized barcode service", moduleID, "initialize barcode service", 0, nil)

	return &barcodeService{
		encoder: &png.Encoder{
			CompressionLevel: png.BestCompression,
		},
	}
}

const (
	moduleIDGenerate = moduleID + ".Generate"
)

// GenerateBarcode encodes data as a barcode image represented by a bytes Buffer
func (b *barcodeService) GenerateBarcode(transport int, data string) (*bytes.Buffer, *governor.Error) {
	switch transport {
	case TransportQRCode:
		qrCode, err := qr.Encode(data, qr.H, qr.Unicode)
		if err != nil {
			return nil, governor.NewError(moduleIDGenerate, err.Error(), 0, http.StatusInternalServerError)
		}

		qrCode, err = bar.Scale(qrCode, barQRSize, barQRSize)
		if err != nil {
			return nil, governor.NewError(moduleIDGenerate, err.Error(), 0, http.StatusInternalServerError)
		}

		buf := &bytes.Buffer{}
		if err := b.encoder.Encode(buf, qrCode); err != nil {
			return nil, governor.NewError(moduleIDGenerate, err.Error(), 0, http.StatusInternalServerError)
		}

		return buf, nil
	}

	return nil, governor.NewError(moduleIDGenerate, "invalid transport", 0, http.StatusInternalServerError)
}
