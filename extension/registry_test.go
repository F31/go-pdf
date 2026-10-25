package extension

import (
	"context"
	"io"
	"reflect"
	"testing"
)

type registryImageFactory struct {
	info ProviderInfo
	encs []ImageEncoding
}

func (f registryImageFactory) Info() ProviderInfo         { return f.info }
func (f registryImageFactory) Features() []Feature        { return nil }
func (f registryImageFactory) Encodings() []ImageEncoding { return f.encs }
func (f registryImageFactory) NewDecoder(context.Context, ImageConfig) (ImageDecoder, error) {
	return testImageDecoder{}, nil
}

type registryColorEngine struct{ info ProviderInfo }

func (e registryColorEngine) Info() ProviderInfo  { return e.info }
func (e registryColorEngine) Features() []Feature { return nil }
func (e registryColorEngine) NewTransform(context.Context, ColorSpec, ColorSpec) (ColorTransform, error) {
	return testColorTransform{}, nil
}

type registryFontEngine struct{ info ProviderInfo }

func (e registryFontEngine) Info() ProviderInfo  { return e.info }
func (e registryFontEngine) Features() []Feature { return nil }
func (e registryFontEngine) Load(context.Context, io.ReaderAt, int64, FontSpec) (FontFace, error) {
	return testFontFace{}, nil
}

func providerInfo(name string) ProviderInfo {
	return ProviderInfo{Name: name, Version: "1", License: "MIT", Determinism: Deterministic}
}

func TestRegistryLookupAndOrdering(t *testing.T) {
	imgA := registryImageFactory{info: providerInfo("jbig2-a"), encs: []ImageEncoding{ImageJBIG2}}
	imgB := registryImageFactory{info: providerInfo("jpx-b"), encs: []ImageEncoding{ImageJPX}}
	color := registryColorEngine{info: providerInfo("lcms")}
	font := registryFontEngine{info: providerInfo("cff")}

	r, err := NewRegistry(
		WithImageFactory(imgB),
		WithImageFactory(imgA),
		WithColorEngine(color),
		WithFontEngine(font),
	)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := r.Encodings(), []ImageEncoding{ImageJBIG2, ImageJPX}; !reflect.DeepEqual(got, want) {
		t.Fatalf("encodings = %v, want %v", got, want)
	}
	if f, ok := r.ImageFactory(ImageJBIG2); !ok || f.Info().Name != "jbig2-a" {
		t.Fatalf("jbig2 factory = %v, %v", f, ok)
	}
	if f, ok := r.ImageFactory("unknown"); ok || f != nil {
		t.Fatalf("unknown factory = %v, %v", f, ok)
	}
	if e, ok := r.ColorEngine("lcms"); !ok || e.Info().Name != "lcms" {
		t.Fatalf("colour engine = %v, %v", e, ok)
	}
	if e, ok := r.FontEngine("cff"); !ok || e.Info().Name != "cff" {
		t.Fatalf("font engine = %v, %v", e, ok)
	}
	if got, want := r.ColorEngineNames(), []string{"lcms"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("colour names = %v, want %v", got, want)
	}
	if got, want := r.FontEngineNames(), []string{"cff"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("font names = %v, want %v", got, want)
	}
	providers := r.Providers()
	if len(providers) != 4 {
		t.Fatalf("providers = %v", providers)
	}
	for i := 1; i < len(providers); i++ {
		if providers[i-1].Name > providers[i].Name {
			t.Fatalf("providers not sorted: %v", providers)
		}
	}
}

func TestRegistryRejectsAmbiguity(t *testing.T) {
	a := registryImageFactory{info: providerInfo("a"), encs: []ImageEncoding{ImageJBIG2, ImageJPX}}
	b := registryImageFactory{info: providerInfo("b"), encs: []ImageEncoding{ImageJPX}}
	if _, err := NewRegistry(WithImageFactory(a), WithImageFactory(b)); err == nil {
		t.Fatal("expected duplicate encoding error")
	}
	if _, err := NewRegistry(WithColorEngine(registryColorEngine{info: providerInfo("c")}), WithColorEngine(registryColorEngine{info: providerInfo("c")})); err == nil {
		t.Fatal("expected duplicate colour engine error")
	}
	if _, err := NewRegistry(WithFontEngine(registryFontEngine{info: providerInfo("f")}), WithFontEngine(registryFontEngine{info: providerInfo("f")})); err == nil {
		t.Fatal("expected duplicate font engine error")
	}
}

func TestRegistryValidatesProviders(t *testing.T) {
	bad := providerInfo("bad")
	bad.Version = ""
	if _, err := NewRegistry(WithImageFactory(registryImageFactory{info: bad, encs: []ImageEncoding{ImageJPX}})); err == nil {
		t.Fatal("expected missing version error")
	}
	if _, err := NewRegistry(WithImageFactory(nil)); err == nil {
		t.Fatal("expected nil factory error")
	}
	if _, err := NewRegistry(WithColorEngine(nil)); err == nil {
		t.Fatal("expected nil engine error")
	}
	if _, err := NewRegistry(nil); err == nil {
		t.Fatal("expected nil option error")
	}
}

func TestNilRegistry(t *testing.T) {
	var r *Registry
	if _, ok := r.ImageFactory(ImageJPX); ok {
		t.Fatal("nil registry returned an image factory")
	}
	if len(r.Encodings()) != 0 || len(r.Providers()) != 0 {
		t.Fatal("nil registry returned data")
	}
}
