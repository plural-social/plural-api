package media

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/nicheinc/expect"
)

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	expect.ErrorNil(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func decodeJPEG(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := jpeg.Decode(bytes.NewReader(data))
	expect.ErrorNil(t, err)
	return img
}

func Test_ProcessImage(t *testing.T) {
	type testCase struct {
		input             []byte
		errorCheck        expect.ErrorCheck
		expectedDisplay   image.Point
		expectedThumbnail image.Point
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			processed, err := ProcessImage(testCase.input)
			testCase.errorCheck(t, err)
			if err != nil {
				return
			}
			expect.Equal(t, image.Pt(processed.Width, processed.Height), testCase.expectedDisplay)
			expect.Equal(t, decodeJPEG(t, processed.Display).Bounds().Size(), testCase.expectedDisplay)
			expect.Equal(t, decodeJPEG(t, processed.Thumbnail).Bounds().Size(), testCase.expectedThumbnail)
		})
	}

	run("Garbage", testCase{
		input:      []byte("not an image"),
		errorCheck: expect.ErrorIs(ErrInvalidImage),
	})
	t.Run("TooManyPixels", func(t *testing.T) {
		original := maxImagePixels
		maxImagePixels = 100
		t.Cleanup(func() { maxImagePixels = original })
		_, err := ProcessImage(encodePNG(t, image.NewRGBA(image.Rect(0, 0, 20, 20))))
		expect.ErrorIs(ErrInvalidImage)(t, err)
	})
	run("LargeLandscape", testCase{
		input:             encodePNG(t, image.NewRGBA(image.Rect(0, 0, 4000, 3000))),
		errorCheck:        expect.ErrorNil,
		expectedDisplay:   image.Pt(2048, 1536),
		expectedThumbnail: image.Pt(480, 360),
	})
	run("SmallImage", testCase{
		input:             encodePNG(t, image.NewRGBA(image.Rect(0, 0, 100, 50))),
		errorCheck:        expect.ErrorNil,
		expectedDisplay:   image.Pt(100, 50),
		expectedThumbnail: image.Pt(100, 50),
	})
	run("Portrait", testCase{
		input:      encodePNG(t, image.NewRGBA(image.Rect(0, 0, 1000, 3000))),
		errorCheck: expect.ErrorNil,
		// imaging.Fit truncates: 1000 × 2048/3000 = 682.67 → 682.
		expectedDisplay:   image.Pt(682, 2048),
		expectedThumbnail: image.Pt(159, 480),
	})
	t.Run("TransparentPNG", func(t *testing.T) {
		// NewRGBA is fully transparent: it must come out white, not black.
		processed, err := ProcessImage(encodePNG(t, image.NewRGBA(image.Rect(0, 0, 10, 10))))
		expect.ErrorNil(t, err)
		r, g, b, _ := decodeJPEG(t, processed.Display).At(0, 0).RGBA()
		white := color.White
		wr, wg, wb, _ := white.RGBA()
		// JPEG is lossy; allow a tiny difference from pure white.
		const tolerance = 0x0300
		for _, pair := range [][2]uint32{{r, wr}, {g, wg}, {b, wb}} {
			expect.Equal(t, pair[1]-pair[0] <= tolerance, true)
		}
	})
}
