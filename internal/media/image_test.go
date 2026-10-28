package media

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

func pngBytes(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestProcessSizes(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 3000, 2000)) // fully transparent
	vs, err := Process(bytes.NewReader(pngBytes(t, src)))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][2]int{"thumb": {320, 213}, "medium": {1080, 720}, "full": {2048, 1365}}
	if len(vs) != len(want) {
		t.Fatalf("got %d variants", len(vs))
	}
	for _, v := range vs {
		img, err := jpeg.Decode(bytes.NewReader(v.JPEG))
		if err != nil {
			t.Fatalf("%s: %v", v.Name, err)
		}
		b := img.Bounds()
		if w := want[v.Name]; b.Dx() != w[0] || b.Dy() != w[1] || v.Width != w[0] || v.Height != w[1] {
			t.Errorf("%s = %dx%d (reported %dx%d), want %v", v.Name, b.Dx(), b.Dy(), v.Width, v.Height, w)
		}
		if r, g, bl, _ := img.At(10, 10).RGBA(); r>>8 < 250 || g>>8 < 250 || bl>>8 < 250 {
			t.Errorf("%s: transparent pixel not white: %v", v.Name, img.At(10, 10))
		}
	}
}

func TestProcessNoUpscale(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 100, 400))
	src.Set(0, 0, color.White)
	vs, err := Process(bytes.NewReader(pngBytes(t, src)))
	if err != nil {
		t.Fatal(err)
	}
	if vs[0].Width != 80 || vs[0].Height != 320 || vs[2].Width != 100 || vs[2].Height != 400 {
		t.Errorf("variants = %+v %+v", vs[0], vs[2])
	}
}

func TestProcessRejects(t *testing.T) {
	// A valid 1x1 PNG whose header claims a 100000x100000 canvas.
	bomb := pngBytes(t, image.NewGray(image.Rect(0, 0, 1, 1)))
	binary.BigEndian.PutUint32(bomb[16:], 100000)
	binary.BigEndian.PutUint32(bomb[20:], 100000)
	binary.BigEndian.PutUint32(bomb[29:], crc32.ChecksumIEEE(bomb[12:29]))

	cases := map[string][]byte{
		"text":  []byte("not an image at all"),
		"bomb":  bomb,
		"empty": nil,
		"huge":  []byte(strings.Repeat("x", MaxBytes+1)),
	}
	for name, data := range cases {
		if _, err := Process(bytes.NewReader(data)); !errors.Is(err, ErrUnsupported) {
			t.Errorf("%s: err = %v, want ErrUnsupported", name, err)
		}
	}
}
