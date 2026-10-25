package extension

import (
	"context"
	"io"
	"testing"
)

type testProvider struct{}

func (testProvider) Info() ProviderInfo {
	return ProviderInfo{Name: "test", Version: "1", License: "MIT", Determinism: Deterministic}
}
func (testProvider) Features() []Feature {
	return []Feature{{Name: "test", Status: StatusSupported}}
}

type testImageFactory struct{ testProvider }

func (testImageFactory) Encodings() []ImageEncoding { return []ImageEncoding{ImageJBIG2} }
func (testImageFactory) NewDecoder(context.Context, ImageConfig) (ImageDecoder, error) {
	return testImageDecoder{}, nil
}

type testImageDecoder struct{}

func (testImageDecoder) Decode(context.Context, io.ReaderAt, int64, ImageConfig) (*ImagePlane, error) {
	return &ImagePlane{}, nil
}
func (testImageDecoder) Close() error { return nil }

type testColorEngine struct{ testProvider }

func (testColorEngine) NewTransform(context.Context, ColorSpec, ColorSpec) (ColorTransform, error) {
	return testColorTransform{}, nil
}

type testColorTransform struct{}

func (testColorTransform) SourceComponents() int      { return 4 }
func (testColorTransform) DestinationComponents() int { return 3 }
func (testColorTransform) Transform(context.Context, []float32, []float32, int) error {
	return nil
}
func (testColorTransform) Close() error { return nil }

type testFontEngine struct{ testProvider }

func (testFontEngine) Load(context.Context, io.ReaderAt, int64, FontSpec) (FontFace, error) {
	return testFontFace{}, nil
}

type testFontFace struct{}

func (testFontFace) Metrics() FontMetrics { return FontMetrics{UnitsPerEm: 1000} }
func (testFontFace) Glyph(context.Context, uint32, uint32) (Glyph, bool, error) {
	return Glyph{GID: 1}, true, nil
}
func (testFontFace) Close() error { return nil }

var (
	_ ImageDecoderFactory = testImageFactory{}
	_ ColorEngine         = testColorEngine{}
	_ FontEngine          = testFontEngine{}
)

func TestProviderStatusVocabulary(t *testing.T) {
	statuses := []Status{
		StatusSupported, StatusApproximate, StatusExternal, StatusUnsupported,
		StatusUnknown, StatusNotScanned, StatusNotApplicable,
	}
	for _, s := range statuses {
		if s == "" {
			t.Fatal("empty status")
		}
	}
}
