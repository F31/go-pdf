package pdf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/extension"
	pdfimage "github.com/F31/go-pdf/internal/image"
)

// WithExtensionRegistry attaches an immutable extension provider catalogue.
// Image providers are consulted only when the legacy DecoderRegistry does not
// handle the encoding; configuring both for the same encoding is rejected.
// DecodeSamples consumes the provider's normalized source samples. The older
// Decode path still adapts planes to image.Image and therefore does not retain
// an ICCBased/CMYK source profile.
func WithExtensionRegistry(r *extension.Registry) Option {
	return func(o *options) error {
		o.extensions = r
		return nil
	}
}

func (o options) validateExtensions() error {
	if o.decoders == nil || o.extensions == nil {
		return nil
	}
	for _, enc := range o.extensions.Encodings() {
		if o.decoders.hasEncoding(displaylist.ImageEncoding(enc)) {
			return fmt.Errorf("pdf: image encoding %q configured in both decoder and extension registries", enc)
		}
	}
	return nil
}

func extensionDecoderLookup(r *extension.Registry) pdfimage.DecoderLookup {
	if r == nil {
		return nil
	}
	return func(ctx context.Context, enc displaylist.ImageEncoding, info displaylist.ImageInfo) (pdfimage.ExternalDecoder, error) {
		factory, ok := r.ImageFactory(extension.ImageEncoding(enc))
		if !ok {
			return nil, nil
		}
		return extensionExternalDecoder{factory: factory}, nil
	}
}

type extensionExternalDecoder struct {
	factory extension.ImageDecoderFactory
}

func (d extensionExternalDecoder) Decode(ctx context.Context, raw []byte, info displaylist.ImageInfo) (image.Image, error) {
	plane, err := d.decodePlane(ctx, raw, info)
	if err != nil {
		return nil, err
	}
	return imagePlaneToImage(plane)
}

func (d extensionExternalDecoder) DecodeSamples(ctx context.Context, raw []byte, info displaylist.ImageInfo) (*displaylist.ImageSamples, error) {
	plane, err := d.decodePlane(ctx, raw, info)
	if err != nil {
		return nil, err
	}
	return extensionPlaneSamples(plane, info)
}

func (d extensionExternalDecoder) decodePlane(ctx context.Context, raw []byte, info displaylist.ImageInfo) (*extension.ImagePlane, error) {
	cfg := extension.ImageConfig{
		Encoding:         extension.ImageEncoding(info.Encoding),
		Width:            info.Width,
		Height:           info.Height,
		ColorSpace:       info.ColorSpace,
		Decode:           append([]float64(nil), info.Decode...),
		ImageMask:        info.ColorSpace == "ImageMask",
		Interpolate:      info.Interpolate,
		BitsPerComponent: info.BitsPerComponent,
	}
	decoder, err := d.factory.NewDecoder(ctx, cfg)
	if err != nil {
		return nil, err
	}
	plane, decodeErr := decoder.Decode(ctx, bytes.NewReader(raw), int64(len(raw)), cfg)
	closeErr := decoder.Close()
	if decodeErr != nil {
		return nil, decodeErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return plane, nil
}

func chainDecoderLookups(first, second pdfimage.DecoderLookup) pdfimage.DecoderLookup {
	if first == nil {
		return second
	}
	if second == nil {
		return first
	}
	return func(ctx context.Context, enc displaylist.ImageEncoding, info displaylist.ImageInfo) (pdfimage.ExternalDecoder, error) {
		d, err := first(ctx, enc, info)
		if err != nil || d != nil {
			return d, err
		}
		return second(ctx, enc, info)
	}
}

func imagePlaneToImage(p *extension.ImagePlane) (image.Image, error) {
	if p == nil {
		return nil, errors.New("pdf: extension image returned nil plane")
	}
	if p.Width <= 0 || p.Height <= 0 {
		return nil, fmt.Errorf("pdf: invalid extension image size %dx%d", p.Width, p.Height)
	}
	if p.Components != 1 && p.Components != 2 && p.Components != 3 && p.Components != 4 {
		return nil, fmt.Errorf("pdf: unsupported extension image components %d", p.Components)
	}
	if p.Components == 2 && !p.Alpha {
		return nil, errors.New("pdf: two-component extension image requires alpha")
	}
	if p.BitsPerComponent != 1 && p.BitsPerComponent != 2 && p.BitsPerComponent != 4 && p.BitsPerComponent != 8 && p.BitsPerComponent != 16 {
		return nil, fmt.Errorf("pdf: unsupported extension image bits per component %d", p.BitsPerComponent)
	}
	bytesPerSample := 1
	if p.BitsPerComponent == 16 {
		bytesPerSample = 2
	}
	rowBytes64 := int64(p.Width) * int64(p.Components) * int64(bytesPerSample)
	if rowBytes64 <= 0 || rowBytes64 > int64(math.MaxInt) {
		return nil, errors.New("pdf: extension image row is too large")
	}
	rowBytes := int(rowBytes64)
	if p.Stride < rowBytes {
		return nil, fmt.Errorf("pdf: extension image stride %d is smaller than row %d", p.Stride, rowBytes)
	}
	if int64(p.Height-1)*int64(p.Stride)+int64(rowBytes) > int64(len(p.Pix)) {
		return nil, errors.New("pdf: extension image pixel buffer is truncated")
	}
	pixels := int64(p.Width) * int64(p.Height)
	if pixels > int64(math.MaxInt)/4 {
		return nil, errors.New("pdf: extension image is too large")
	}

	sample := func(x, y, c int) uint32 {
		off := y*p.Stride + (x*p.Components+c)*bytesPerSample
		if bytesPerSample == 2 {
			return uint32(p.Pix[off])<<8 | uint32(p.Pix[off+1])
		}
		return uint32(p.Pix[off])
	}
	maxv := uint32((1 << p.BitsPerComponent) - 1)
	if p.BitsPerComponent == 16 {
		maxv = 65535
	}
	scale8 := func(v uint32) uint8 {
		if p.BitsPerComponent == 16 {
			return uint8(v >> 8)
		}
		if maxv == 255 {
			return uint8(v)
		}
		return uint8((v*255 + maxv/2) / maxv)
	}

	switch {
	case p.Components == 1 && p.BitsPerComponent == 16:
		out := image.NewGray16(image.Rect(0, 0, p.Width, p.Height))
		for y := 0; y < p.Height; y++ {
			for x := 0; x < p.Width; x++ {
				out.SetGray16(x, y, color.Gray16{Y: uint16(sample(x, y, 0))})
			}
		}
		return out, nil
	case p.Components == 1:
		out := image.NewGray(image.Rect(0, 0, p.Width, p.Height))
		for y := 0; y < p.Height; y++ {
			for x := 0; x < p.Width; x++ {
				out.SetGray(x, y, color.Gray{Y: scale8(sample(x, y, 0))})
			}
		}
		return out, nil
	case p.Components == 2:
		out := image.NewNRGBA(image.Rect(0, 0, p.Width, p.Height))
		for y := 0; y < p.Height; y++ {
			for x := 0; x < p.Width; x++ {
				g := scale8(sample(x, y, 0))
				out.SetNRGBA(x, y, color.NRGBA{R: g, G: g, B: g, A: scale8(sample(x, y, 1))})
			}
		}
		return out, nil
	case p.Components == 3 && p.BitsPerComponent == 16:
		out := image.NewRGBA64(image.Rect(0, 0, p.Width, p.Height))
		for y := 0; y < p.Height; y++ {
			for x := 0; x < p.Width; x++ {
				out.SetRGBA64(x, y, color.RGBA64{
					R: uint16(sample(x, y, 0)), G: uint16(sample(x, y, 1)), B: uint16(sample(x, y, 2)), A: 65535,
				})
			}
		}
		return out, nil
	case p.Components == 3:
		out := image.NewNRGBA(image.Rect(0, 0, p.Width, p.Height))
		for y := 0; y < p.Height; y++ {
			for x := 0; x < p.Width; x++ {
				out.SetNRGBA(x, y, color.NRGBA{R: scale8(sample(x, y, 0)), G: scale8(sample(x, y, 1)), B: scale8(sample(x, y, 2)), A: 255})
			}
		}
		return out, nil
	case p.Alpha && p.BitsPerComponent == 16:
		if p.Premultiplied {
			out := image.NewRGBA64(image.Rect(0, 0, p.Width, p.Height))
			for y := 0; y < p.Height; y++ {
				for x := 0; x < p.Width; x++ {
					out.SetRGBA64(x, y, color.RGBA64{
						R: uint16(sample(x, y, 0)), G: uint16(sample(x, y, 1)), B: uint16(sample(x, y, 2)), A: uint16(sample(x, y, 3)),
					})
				}
			}
			return out, nil
		}
		out := image.NewNRGBA64(image.Rect(0, 0, p.Width, p.Height))
		for y := 0; y < p.Height; y++ {
			for x := 0; x < p.Width; x++ {
				out.SetNRGBA64(x, y, color.NRGBA64{
					R: uint16(sample(x, y, 0)), G: uint16(sample(x, y, 1)), B: uint16(sample(x, y, 2)), A: uint16(sample(x, y, 3)),
				})
			}
		}
		return out, nil
	case p.Alpha && p.Premultiplied:
		out := image.NewRGBA(image.Rect(0, 0, p.Width, p.Height))
		for y := 0; y < p.Height; y++ {
			for x := 0; x < p.Width; x++ {
				out.SetRGBA(x, y, color.RGBA{
					R: scale8(sample(x, y, 0)), G: scale8(sample(x, y, 1)), B: scale8(sample(x, y, 2)), A: scale8(sample(x, y, 3)),
				})
			}
		}
		return out, nil
	case p.Alpha:
		out := image.NewNRGBA(image.Rect(0, 0, p.Width, p.Height))
		for y := 0; y < p.Height; y++ {
			for x := 0; x < p.Width; x++ {
				out.SetNRGBA(x, y, color.NRGBA{
					R: scale8(sample(x, y, 0)), G: scale8(sample(x, y, 1)), B: scale8(sample(x, y, 2)), A: scale8(sample(x, y, 3)),
				})
			}
		}
		return out, nil
	case p.BitsPerComponent == 16:
		return nil, errors.New("pdf: 16-bit CMYK extension images are unsupported")
	default:
		out := image.NewCMYK(image.Rect(0, 0, p.Width, p.Height))
		for y := 0; y < p.Height; y++ {
			for x := 0; x < p.Width; x++ {
				out.SetCMYK(x, y, color.CMYK{
					C: scale8(sample(x, y, 0)), M: scale8(sample(x, y, 1)), Y: scale8(sample(x, y, 2)), K: scale8(sample(x, y, 3)),
				})
			}
		}
		return out, nil
	}
}

func extensionPlaneSamples(p *extension.ImagePlane, info displaylist.ImageInfo) (*displaylist.ImageSamples, error) {
	if p == nil {
		return nil, errors.New("pdf: extension image returned nil plane")
	}
	if p.Alpha {
		return nil, errors.New("pdf: alpha extension planes cannot be exposed as source colour samples")
	}
	if p.Width <= 0 || p.Height <= 0 {
		return nil, fmt.Errorf("pdf: invalid extension image size %dx%d", p.Width, p.Height)
	}
	if p.Components != 1 && p.Components != 3 && p.Components != 4 {
		return nil, fmt.Errorf("pdf: unsupported extension sample components %d", p.Components)
	}
	if p.BitsPerComponent != 1 && p.BitsPerComponent != 2 && p.BitsPerComponent != 4 && p.BitsPerComponent != 8 && p.BitsPerComponent != 16 {
		return nil, fmt.Errorf("pdf: unsupported extension sample bits per component %d", p.BitsPerComponent)
	}
	bytesPerSample := 1
	if p.BitsPerComponent == 16 {
		bytesPerSample = 2
	}
	rowBytes64 := int64(p.Width) * int64(p.Components) * int64(bytesPerSample)
	if rowBytes64 <= 0 || rowBytes64 > int64(math.MaxInt) {
		return nil, errors.New("pdf: extension sample row is too large")
	}
	rowBytes := int(rowBytes64)
	if p.Stride < rowBytes {
		return nil, fmt.Errorf("pdf: extension sample stride %d is smaller than row %d", p.Stride, rowBytes)
	}
	if int64(p.Height-1)*int64(p.Stride)+int64(rowBytes) > int64(len(p.Pix)) {
		return nil, errors.New("pdf: extension sample buffer is truncated")
	}
	pixels := int64(p.Width) * int64(p.Height)
	if pixels > int64(math.MaxInt)/4 {
		return nil, errors.New("pdf: extension samples are too large")
	}
	sample := func(x, y, c int) uint32 {
		off := y*p.Stride + (x*p.Components+c)*bytesPerSample
		if bytesPerSample == 2 {
			return uint32(p.Pix[off])<<8 | uint32(p.Pix[off+1])
		}
		return uint32(p.Pix[off])
	}
	maxv := uint32((1 << p.BitsPerComponent) - 1)
	if p.BitsPerComponent == 16 {
		maxv = 65535
	}
	space := info.ColorSpace
	if space == "" {
		space = p.ColorSpace
	}
	out := make([]float32, int(pixels)*p.Components)
	for y := 0; y < p.Height; y++ {
		for x := 0; x < p.Width; x++ {
			for c := 0; c < p.Components; c++ {
				v := float64(sample(x, y, c)) / float64(maxv)
				if len(info.Decode) >= 2*c+2 {
					lo, hi := info.Decode[2*c], info.Decode[2*c+1]
					v = lo + v*(hi-lo)
				}
				if v < 0 {
					v = 0
				} else if v > 1 {
					v = 1
				}
				out[(y*p.Width+x)*p.Components+c] = float32(v)
			}
		}
	}
	return displaylist.NewImageSamples(p.Width, p.Height, p.Components, space, nil, out, nil), nil
}
