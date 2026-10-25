package pdf

import (
	"context"
	"io"
	"testing"

	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/extension"
)

type captureCCITTFactory struct {
	cfg extension.ImageConfig
}

func (f *captureCCITTFactory) Info() extension.ProviderInfo {
	return extension.ProviderInfo{Name: "capture", Version: "1", License: "test", Determinism: extension.Deterministic}
}
func (f *captureCCITTFactory) Features() []extension.Feature { return nil }
func (f *captureCCITTFactory) Encodings() []extension.ImageEncoding {
	return []extension.ImageEncoding{extension.ImageCCITTG32D}
}
func (f *captureCCITTFactory) NewDecoder(_ context.Context, cfg extension.ImageConfig) (extension.ImageDecoder, error) {
	f.cfg = cfg
	return captureCCITTDecoder{}, nil
}

type captureCCITTDecoder struct{}

func (captureCCITTDecoder) Decode(context.Context, io.ReaderAt, int64, extension.ImageConfig) (*extension.ImagePlane, error) {
	return &extension.ImagePlane{Pix: []byte{0}, Stride: 1, Width: 1, Height: 1, Components: 1, BitsPerComponent: 8}, nil
}
func (captureCCITTDecoder) Close() error { return nil }

func TestExternalDecoderMapsCCITTParams(t *testing.T) {
	f := &captureCCITTFactory{}
	r, err := extension.NewRegistry(extension.WithImageFactory(f))
	if err != nil {
		t.Fatal(err)
	}
	lookup := extensionDecoderLookup(r)
	info := displaylist.ImageInfo{
		Width: 10, Height: 20, Encoding: displaylist.ImageEncoding(extension.ImageCCITTG32D),
		CCITT: &displaylist.CCITTParams{
			K: 2, Columns: 10, Rows: 20, BlackIsZero: true,
			EncodedByteAlign: true, EndOfLine: true, EndOfBlock: false, EndOfData: true,
		},
	}
	d, err := lookup(context.Background(), displaylist.ImageEncoding(extension.ImageCCITTG32D), info)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Decode(context.Background(), []byte{0}, info); err != nil {
		t.Fatal(err)
	}
	if f.cfg.CCITT == nil || f.cfg.CCITT.K != 2 || f.cfg.CCITT.Columns != 10 ||
		f.cfg.CCITT.Rows != 20 || !f.cfg.CCITT.BlackIsZero ||
		!f.cfg.CCITT.EncodedByteAlign || !f.cfg.CCITT.EndOfLine ||
		f.cfg.CCITT.EndOfBlock || !f.cfg.CCITT.EndOfData {
		t.Fatalf("cfg = %#v", f.cfg.CCITT)
	}
}
