package legacy

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"testing"

	"github.com/F31/go-pdf"
	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/extension"
	"github.com/F31/go-pdf/render"
)

func testInfo(name string) extension.ProviderInfo {
	return extension.ProviderInfo{Name: name, Version: "legacy-1", License: "caller", Determinism: extension.ProviderDependent}
}

type fakeDecoderFactory struct{}

func (fakeDecoderFactory) Encodings() []displaylist.ImageEncoding {
	return []displaylist.ImageEncoding{"JBIG2Decode"}
}
func (fakeDecoderFactory) NewDecoder(context.Context, displaylist.ImageInfo) (pdf.ExternalDecoder, error) {
	return fakeExternalDecoder{}, nil
}

type fakeExternalDecoder struct{}

func (fakeExternalDecoder) Decode(context.Context, []byte, displaylist.ImageInfo) (image.Image, error) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	img.Set(1, 0, color.NRGBA{G: 255, A: 255})
	return img, nil
}

func TestImageFactoryAdapter(t *testing.T) {
	f := NewImageFactory(testInfo("legacy-image"), fakeDecoderFactory{})
	if f == nil {
		t.Fatal("nil adapter")
	}
	if got, want := f.Encodings(), []extension.ImageEncoding{"JBIG2Decode"}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("encodings = %v", got)
	}
	d, err := f.NewDecoder(context.Background(), extension.ImageConfig{})
	if err != nil {
		t.Fatal(err)
	}
	plane, err := d.Decode(context.Background(), bytes.NewReader([]byte{1, 2, 3}), 3, extension.ImageConfig{
		Width: 2, Height: 1, Components: 4, BitsPerComponent: 8, Encoding: extension.ImageJBIG2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plane.Width != 2 || plane.Height != 1 || plane.Components != 4 || plane.BitsPerComponent != 8 {
		t.Fatalf("plane = %#v", plane)
	}
	if plane.ColorSpace != "sRGB" || !plane.Alpha {
		t.Fatalf("plane metadata = %#v", plane)
	}
	if len(plane.Pix) < plane.Stride {
		t.Fatalf("short pix: %d < %d", len(plane.Pix), plane.Stride)
	}
}

type fakeColorManager struct{}

func (fakeColorManager) Convert(info pdf.ColorSpaceInfo, values []float64) ([4]float64, bool, error) {
	return [4]float64{1, 0, 0, 1}, false, nil
}

func TestColorEngineAdapter(t *testing.T) {
	e := NewColorEngine(testInfo("legacy-color"), fakeColorManager{})
	tr, err := e.NewTransform(context.Background(),
		extension.ColorSpec{Name: "DeviceCMYK", Components: 4},
		extension.ColorSpec{Name: "sRGB", Components: 3})
	if err != nil {
		t.Fatal(err)
	}
	src := []float32{0, 0, 0, 0}
	dst := make([]float32, 3)
	if err := tr.Transform(context.Background(), dst, src, 1); err != nil {
		t.Fatal(err)
	}
	if dst[0] != 1 || dst[1] != 0 || dst[2] != 0 {
		t.Fatalf("dst = %v", dst)
	}
	if _, err := e.NewTransform(context.Background(), extension.ColorSpec{Name: "DeviceRGB", Components: 3}, extension.ColorSpec{Name: "DeviceCMYK", Components: 4}); err == nil {
		t.Fatal("expected unsupported destination error")
	}
}

type fakeFontProvider struct{}

func (fakeFontProvider) GlyphOutline(info *displaylist.FontInfo, glyph displaylist.Glyph) ([]displaylist.Segment, bool, error) {
	if info.BaseFont == "" {
		return nil, false, nil
	}
	return []displaylist.Segment{{Kind: displaylist.SegmentLine, P0: displaylist.Point{X: 1, Y: 2}, P1: displaylist.Point{X: 3, Y: 4}}}, true, nil
}

var _ render.FontProvider = fakeFontProvider{}

func TestFontEngineAdapter(t *testing.T) {
	e := NewFontEngine(testInfo("legacy-font"), fakeFontProvider{})
	face, err := e.Load(context.Background(), nil, 0, extension.FontSpec{BaseFont: "Test", Subtype: "Type1", Embedded: true})
	if err != nil {
		t.Fatal(err)
	}
	g, ok, err := face.Glyph(context.Background(), 7, 9)
	if err != nil || !ok {
		t.Fatalf("glyph = %v %v %v", g, ok, err)
	}
	if g.Code != 7 || g.CID != 9 || len(g.Outline) != 1 || g.Outline[0].Op != extension.OutlineLine {
		t.Fatalf("glyph = %#v", g)
	}
	if g.Outline[0].P0.X != 1 || g.Outline[0].P1.Y != 4 {
		t.Fatalf("outline = %#v", g.Outline)
	}
}

func TestAdaptersRejectNilLegacy(t *testing.T) {
	if NewImageFactory(testInfo("x"), nil) != nil {
		t.Fatal("nil image factory accepted")
	}
	if NewColorEngine(testInfo("x"), nil) != nil {
		t.Fatal("nil color manager accepted")
	}
	if NewFontEngine(testInfo("x"), nil) != nil {
		t.Fatal("nil font provider accepted")
	}
}

func TestAdaptersBuildRegistry(t *testing.T) {
	r, err := extension.NewRegistry(
		extension.WithImageFactory(NewImageFactory(testInfo("legacy-image"), fakeDecoderFactory{})),
		extension.WithColorEngine(NewColorEngine(testInfo("legacy-color"), fakeColorManager{})),
		extension.WithFontEngine(NewFontEngine(testInfo("legacy-font"), fakeFontProvider{})),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.ImageFactory(extension.ImageJBIG2); !ok {
		t.Fatal("image adapter not registered")
	}
	if _, ok := r.ColorEngine("legacy-color"); !ok {
		t.Fatal("colour adapter not registered")
	}
	if _, ok := r.FontEngine("legacy-font"); !ok {
		t.Fatal("font adapter not registered")
	}
}
