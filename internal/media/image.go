// Package media turns an uploaded image into the sizes clients display.
package media

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"

	// Accepted upload formats. The format is detected from the bytes, never the name.
	_ "image/gif"
	_ "image/png"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// MaxBytes caps an upload. The API rejects bigger objects before processing.
const MaxBytes = 20 << 20

// maxPixels guards against decompression bombs: a tiny file can declare a
// huge canvas, and decoding allocates width*height*4 bytes up front.
const maxPixels = 50_000_000

// Sizes are the longest-side bounds of each variant. Smaller images are not upscaled.
var Sizes = []struct {
	Name string
	Max  int
}{
	{"thumb", 320},
	{"medium", 1080},
	{"full", 2048},
}

var ErrUnsupported = errors.New("media: not a supported image")

type Variant struct {
	Name          string
	Width, Height int
	JPEG          []byte
}

// Process decodes an image and re-encodes every size as JPEG. Re-encoding
// drops all metadata, EXIF GPS included.
// shortcut: EXIF orientation is ignored, so phone photos may come out rotated; read the tag when clients complain.
func Process(r io.Reader) ([]Variant, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("media: read: %w", err)
	}
	if len(data) > MaxBytes {
		return nil, fmt.Errorf("%w: larger than %d bytes", ErrUnsupported, MaxBytes)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupported, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxPixels {
		return nil, fmt.Errorf("%w: %dx%d", ErrUnsupported, cfg.Width, cfg.Height)
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupported, err)
	}

	out := make([]Variant, 0, len(Sizes))
	for _, s := range Sizes {
		w, h := fit(cfg.Width, cfg.Height, s.Max)
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		// JPEG has no alpha; transparent areas become white instead of black.
		draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 82}); err != nil {
			return nil, fmt.Errorf("media: encode %s: %w", s.Name, err)
		}
		out = append(out, Variant{Name: s.Name, Width: w, Height: h, JPEG: buf.Bytes()})
	}
	return out, nil
}

// fit scales w×h down so the longest side is at most bound, keeping the ratio.
func fit(w, h, bound int) (int, int) {
	if w <= bound && h <= bound {
		return w, h
	}
	if w >= h {
		return bound, max(1, h*bound/w)
	}
	return max(1, w*bound/h), bound
}
