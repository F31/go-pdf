package g32d

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/F31/go-pdf/extension"
)

const (
	maxPixels     = 64_000_000
	maxDimension  = 1 << 20
	maxInputBytes = 64 << 20
)

// Factory is the optional pure-Go Group 3 2-D image provider.
type Factory struct{}

// New returns a stateless G3 2-D provider.
func New() *Factory { return &Factory{} }

func (f *Factory) Info() extension.ProviderInfo {
	return extension.ProviderInfo{
		Name: "g32d", Version: "go-pdf-v1", License: "Apache-2.0",
		Determinism: extension.Deterministic,
	}
}

func (f *Factory) Features() []extension.Feature {
	return []extension.Feature{{
		Name: "ccitt-g3-2d", Status: extension.StatusSupported,
		Reason: "pure-Go provider; explicit registration required",
	}}
}

func (f *Factory) Encodings() []extension.ImageEncoding {
	return []extension.ImageEncoding{extension.ImageCCITTG32D}
}

func (f *Factory) NewDecoder(context.Context, extension.ImageConfig) (extension.ImageDecoder, error) {
	return &decoder{}, nil
}

type decoder struct{}

func (d *decoder) Close() error { return nil }

func (d *decoder) Decode(ctx context.Context, src io.ReaderAt, size int64, cfg extension.ImageConfig) (*extension.ImagePlane, error) {
	if src == nil || size <= 0 {
		return nil, errors.New("g32d: empty CCITT stream")
	}
	if size > maxInputBytes {
		return nil, errors.New("g32d: CCITT stream exceeds input limit")
	}
	if cfg.CCITT == nil || cfg.CCITT.K <= 0 {
		return nil, errors.New("g32d: provider requires K > 0")
	}
	width, height := cfg.Width, cfg.Height
	if cfg.CCITT.Columns > 0 {
		width = cfg.CCITT.Columns
	}
	if cfg.CCITT.Rows > 0 {
		height = cfg.CCITT.Rows
	}
	if width <= 0 || height <= 0 || width > maxDimension || height > maxDimension {
		return nil, errors.New("g32d: invalid image dimensions")
	}
	if int64(width)*int64(height) > maxPixels {
		return nil, errors.New("g32d: image exceeds pixel limit")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw := make([]byte, size)
	for off := int64(0); off < size; {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, err := src.ReadAt(raw[off:], off)
		off += int64(n)
		if err != nil {
			if err == io.EOF && off == size {
				break
			}
			return nil, err
		}
		if n == 0 {
			return nil, io.ErrNoProgress
		}
	}
	r := NewReader(bytes.NewReader(raw), MSB, Group3_2D, width, height, &Options{
		Align: cfg.CCITT.EncodedByteAlign,
	})
	rowBytes := (width + 7) / 8
	bits := make([]byte, rowBytes*height)
	if err := readFullContext(ctx, r, bits); err != nil {
		return nil, fmt.Errorf("g32d: decode: %w", err)
	}
	return planeFromBits(bits, width, height, cfg), nil
}

func readFullContext(ctx context.Context, r io.Reader, dst []byte) error {
	for len(dst) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		n := len(dst)
		if n > 32*1024 {
			n = 32 * 1024
		}
		read, err := r.Read(dst[:n])
		dst = dst[read:]
		if err != nil {
			if err == io.EOF && len(dst) == 0 {
				return nil
			}
			return err
		}
		if read == 0 {
			return io.ErrNoProgress
		}
	}
	return nil
}

func planeFromBits(bits []byte, width, height int, cfg extension.ImageConfig) *extension.ImagePlane {
	rowBytes := (width + 7) / 8
	if cfg.ImageMask {
		pix := make([]byte, width*height*2)
		for y := 0; y < height; y++ {
			row := bits[y*rowBytes:]
			for x := 0; x < width; x++ {
				v := sample(row[x/8], x, cfg)
				off := (y*width + x) * 2
				pix[off] = 0
				pix[off+1] = uint8(255 - int(v*255+0.5))
			}
		}
		return &extension.ImagePlane{
			Pix: pix, Stride: width * 2, Width: width, Height: height,
			Components: 2, BitsPerComponent: 8, ColorSpace: "DeviceGray", Alpha: true,
		}
	}
	pix := make([]byte, width*height)
	for y := 0; y < height; y++ {
		row := bits[y*rowBytes:]
		for x := 0; x < width; x++ {
			pix[y*width+x] = uint8(sample(row[x/8], x, cfg)*255 + 0.5)
		}
	}
	return &extension.ImagePlane{
		Pix: pix, Stride: width, Width: width, Height: height,
		Components: 1, BitsPerComponent: 8, ColorSpace: "DeviceGray",
	}
}

func sample(b byte, x int, cfg extension.ImageConfig) float64 {
	bit := (b >> uint(7-x%8)) & 1
	v := float64(bit)
	if cfg.CCITT != nil && cfg.CCITT.BlackIsZero {
		v = 1 - v
	}
	if len(cfg.Decode) >= 2 {
		v = cfg.Decode[0] + v*(cfg.Decode[1]-cfg.Decode[0])
	}
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
