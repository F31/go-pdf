package image

import (
	"context"
	"image/color"
	"testing"

	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/internal/syntax"
)

func FuzzImageDecode(f *testing.F) {
	f.Add([]byte{255, 0, 0}, uint8(1), uint8(1), uint8(3), uint8(1))
	f.Add([]byte{0, 0, 0, 0}, uint8(1), uint8(1), uint8(4), uint8(2))
	f.Add([]byte{0x40}, uint8(2), uint8(1), uint8(0), uint8(3))
	f.Add([]byte{}, uint8(0), uint8(0), uint8(0), uint8(0))
	f.Fuzz(func(t *testing.T, data []byte, w, h, bpc, cs uint8) {
		if w == 0 || h == 0 || w > 64 || h > 64 {
			return
		}
		bits := []int{1, 2, 4, 8, 16}[int(bpc)%5]
		spaces := []string{"DeviceGray", "DeviceRGB", "DeviceCMYK", "ImageMask", "Indexed"}
		r := &Ref{info: displaylist.ImageInfo{
			Width: int(w), Height: int(h), BitsPerComponent: bits,
			ColorSpace: spaces[int(cs)%len(spaces)],
		}}
		_, _ = r.decodeSamples(data)
	})
}

type testColorManager struct {
	space string
	err   error
}

func (m *testColorManager) Convert(space string, profile []byte, n int, values []float64) ([4]float64, bool, error) {
	m.space = space
	if m.err != nil {
		return [4]float64{}, false, m.err
	}
	return [4]float64{1, 0, 0, 1}, true, nil
}

func TestCMYKImageUsesColorManager(t *testing.T) {
	cm := &testColorManager{}
	r := &Ref{
		info:         displaylist.ImageInfo{Width: 1, Height: 1, BitsPerComponent: 8, ColorSpace: "DeviceCMYK"},
		colorManager: cm,
	}
	img, err := r.decodeSamples([]byte{0, 0, 0, 0})
	if err != nil {
		t.Fatal(err)
	}
	if cm.space != "DeviceCMYK" {
		t.Fatalf("space = %q", cm.space)
	}
	if got := img.At(0, 0); got != (color.RGBA{R: 255, A: 255}) {
		t.Fatalf("pixel = %#v", got)
	}
}

func TestICCBasedImageUsesColorManager(t *testing.T) {
	cm := &testColorManager{}
	r := &Ref{
		info:       displaylist.ImageInfo{Width: 1, Height: 1, BitsPerComponent: 8, ColorSpace: "ICCBased"},
		components: 4, iccProfile: []byte{1, 2, 3}, colorManager: cm,
	}
	if _, err := r.decodeSamples([]byte{0, 0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	if cm.space != "ICCBased" {
		t.Fatalf("space = %q", cm.space)
	}
}

func TestDecodeSourceSamples(t *testing.T) {
	r := &Ref{
		info:       displaylist.ImageInfo{Width: 1, Height: 1, BitsPerComponent: 8, ColorSpace: "DeviceCMYK", Decode: []float64{0, 1, 0, 1, 0, 1, 0, 1}},
		raw:        []byte{0, 0, 0, 0},
		iccProfile: []byte{1, 2, 3},
	}
	s, err := r.DecodeSamples(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.Components != 4 || s.Width != 1 || s.Height != 1 {
		t.Fatalf("samples = %#v", s)
	}
	for i, v := range s.Data {
		if v != 0 {
			t.Fatalf("sample %d = %v", i, v)
		}
	}
	if len(s.Profile) != 3 {
		t.Fatalf("profile = %v", s.Profile)
	}
}

func TestImageInfoCCITTParams(t *testing.T) {
	dict := syntax.Dict{
		"Width":  syntax.Int(12),
		"Height": syntax.Int(8),
		"Filter": syntax.Name("CCITTFaxDecode"),
		"DecodeParms": syntax.Dict{
			"K": syntax.Int(2), "Columns": syntax.Int(12), "Rows": syntax.Int(8),
			"BlackIsZero": syntax.Bool(true), "EncodedByteAlign": syntax.Bool(true),
			"EndOfLine": syntax.Bool(true), "EndOfBlock": syntax.Bool(false), "EndOfData": syntax.Bool(true),
		},
	}
	info, _, _, err := imageInfo(context.Background(), dict, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if info.CCITT == nil || info.CCITT.K != 2 || info.CCITT.Columns != 12 || info.CCITT.Rows != 8 ||
		!info.CCITT.BlackIsZero || !info.CCITT.EncodedByteAlign || !info.CCITT.EndOfLine ||
		info.CCITT.EndOfBlock || !info.CCITT.EndOfData {
		t.Fatalf("ccitt = %#v", info.CCITT)
	}
}
