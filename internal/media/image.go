package media

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"

	"github.com/disintegration/imaging"
	_ "golang.org/x/image/webp" // Registers WebP with image.Decode and imaging.
)

var ErrInvalidImage = errors.New("invalid image")

const (
	displayMaxSide   = 2048
	thumbnailMaxSide = 480

	displayJPEGQuality   = 85
	thumbnailJPEGQuality = 80
)

// maxImagePixels guards against decompression bombs: a small file that
// decodes to a huge bitmap. A var so tests can lower it.
var maxImagePixels = 50_000_000

type ProcessedImage struct {
	Display       []byte // JPEG
	Thumbnail     []byte // JPEG
	Width, Height int    // of Display
}

// ProcessImage decodes data (JPEG, PNG or WebP), applies EXIF orientation,
// scales it down to fit displayMaxSide and thumbnailMaxSide (never up), and
// encodes both as JPEG. Re-encoding drops all metadata, including GPS.
func ProcessImage(data []byte) (ProcessedImage, error) {
	// Check dimensions from the header before decoding the full bitmap.
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width*config.Height > maxImagePixels {
		return ProcessedImage{}, ErrInvalidImage
	}

	img, err := imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))
	if err != nil {
		return ProcessedImage{}, ErrInvalidImage
	}
	// JPEG has no alpha channel; without this, transparent areas turn black.
	bounds := img.Bounds()
	img = imaging.Overlay(imaging.New(bounds.Dx(), bounds.Dy(), color.White), img, image.Pt(0, 0), 1)

	display := fit(img, displayMaxSide)
	displayJPEG, err := encodeJPEG(display, displayJPEGQuality)
	if err != nil {
		return ProcessedImage{}, err
	}
	thumbnailJPEG, err := encodeJPEG(fit(display, thumbnailMaxSide), thumbnailJPEGQuality)
	if err != nil {
		return ProcessedImage{}, err
	}

	return ProcessedImage{
		Display:   displayJPEG,
		Thumbnail: thumbnailJPEG,
		Width:     display.Bounds().Dx(),
		Height:    display.Bounds().Dy(),
	}, nil
}

// fit scales img down to fit in a side×side box. It never scales up.
func fit(img image.Image, side int) image.Image {
	bounds := img.Bounds()
	if bounds.Dx() <= side && bounds.Dy() <= side {
		return img
	}
	return imaging.Fit(img, side, side, imaging.Lanczos)
}

func encodeJPEG(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := imaging.Encode(&buf, img, imaging.JPEG, imaging.JPEGQuality(quality)); err != nil {
		return nil, fmt.Errorf("encoding JPEG: %w", err)
	}
	return buf.Bytes(), nil
}
