package legacy

import (
	"context"
	"errors"
	"image"
	"image/draw"
	"io"

	"github.com/F31/go-pdf"
	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/extension"
)

// NewImageFactory adapts a legacy pdf.DecoderFactory to an extension image
// factory. The caller must supply accurate provider metadata.
func NewImageFactory(info extension.ProviderInfo, f pdf.DecoderFactory) extension.ImageDecoderFactory {
	if f == nil {
		return nil
	}
	return imageFactory{info: info, legacy: f}
}

type imageFactory struct {
	info   extension.ProviderInfo
	legacy pdf.DecoderFactory
}

func (f imageFactory) Info() extension.ProviderInfo  { return f.info }
func (f imageFactory) Features() []extension.Feature { return nil }

func (f imageFactory) Encodings() []extension.ImageEncoding {
	encs := f.legacy.Encodings()
	out := make([]extension.ImageEncoding, 0, len(encs))
	for _, enc := range encs {
		out = append(out, extension.ImageEncoding(enc))
	}
	return out
}

func (f imageFactory) NewDecoder(context.Context, extension.ImageConfig) (extension.ImageDecoder, error) {
	return imageDecoder{factory: f.legacy}, nil
}

type imageDecoder struct {
	factory pdf.DecoderFactory
}

func (d imageDecoder) Decode(ctx context.Context, src io.ReaderAt, size int64, cfg extension.ImageConfig) (*extension.ImagePlane, error) {
	if src == nil || size < 0 {
		return nil, errors.New("legacy: invalid image source")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if size > int64(int(^uint(0)>>1)) {
		return nil, errors.New("legacy: image source too large")
	}
	raw := make([]byte, int(size))
	if size > 0 {
		if _, err := io.ReadFull(io.NewSectionReader(src, 0, size), raw); err != nil {
			return nil, err
		}
	}
	info := displaylist.ImageInfo{
		Width:            cfg.Width,
		Height:           cfg.Height,
		BitsPerComponent: cfg.BitsPerComponent,
		ColorSpace:       cfg.ColorSpace,
		Encoding:         displaylist.ImageEncoding(cfg.Encoding),
		Decode:           append([]float64(nil), cfg.Decode...),
		Interpolate:      cfg.Interpolate,
	}
	if cfg.CCITT != nil {
		info.CCITT = &displaylist.CCITTParams{
			K: cfg.CCITT.K, Columns: cfg.CCITT.Columns, Rows: cfg.CCITT.Rows,
			BlackIsZero: cfg.CCITT.BlackIsZero, EncodedByteAlign: cfg.CCITT.EncodedByteAlign,
			EndOfLine: cfg.CCITT.EndOfLine, EndOfBlock: cfg.CCITT.EndOfBlock, EndOfData: cfg.CCITT.EndOfData,
		}
	}
	decoder, err := d.factory.NewDecoder(ctx, info)
	if err != nil {
		return nil, err
	}
	img, err := decoder.Decode(ctx, raw, info)
	if err != nil {
		return nil, err
	}
	if img == nil {
		return nil, errors.New("legacy: decoder returned nil image")
	}
	bounds := img.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return nil, errors.New("legacy: decoder returned an empty image")
	}
	dst := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(dst, dst.Bounds(), img, bounds.Min, draw.Src)
	return &extension.ImagePlane{
		Pix:              dst.Pix,
		Stride:           dst.Stride,
		Width:            bounds.Dx(),
		Height:           bounds.Dy(),
		Components:       4,
		BitsPerComponent: 8,
		ColorSpace:       "sRGB",
		Alpha:            true,
	}, nil
}

func (imageDecoder) Close() error { return nil }
