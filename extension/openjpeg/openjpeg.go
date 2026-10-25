package openjpeg

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/F31/go-pdf/extension"
)

// Factory is an image provider backed by the opj_decompress executable. It is
// immutable and safe for concurrent use.
type Factory struct {
	binary  string
	version string
}

// New locates binary (or opj_decompress when empty) and records its version.
func New(binary string) (*Factory, error) {
	if binary == "" {
		binary = "opj_decompress"
	}
	path, err := exec.LookPath(binary)
	if err != nil {
		return nil, fmt.Errorf("openjpeg: executable %q: %w", binary, err)
	}
	f := &Factory{binary: path, version: "unknown"}
	out, _ := exec.Command(path, "-h").CombinedOutput()
	if line := strings.TrimSpace(string(out)); line != "" {
		if i := strings.IndexByte(line, '\n'); i >= 0 {
			line = line[:i]
		}
		f.version = line
	}
	return f, nil
}

// Info implements extension.Provider.
func (f *Factory) Info() extension.ProviderInfo {
	return extension.ProviderInfo{
		Name:        "openjpeg",
		Version:     f.version,
		License:     "BSD-2-Clause",
		Determinism: extension.ProviderDependent,
	}
}

// Features implements extension.Provider.
func (f *Factory) Features() []extension.Feature {
	return []extension.Feature{{
		Name:   string(extension.ImageJPX),
		Status: extension.StatusExternal,
		Reason: "decoded by the opj_decompress process",
	}}
}

// Encodings implements extension.ImageDecoderFactory.
func (f *Factory) Encodings() []extension.ImageEncoding {
	return []extension.ImageEncoding{extension.ImageJPX}
}

// NewDecoder implements extension.ImageDecoderFactory.
func (f *Factory) NewDecoder(context.Context, extension.ImageConfig) (extension.ImageDecoder, error) {
	if f == nil {
		return nil, errors.New("openjpeg: nil factory")
	}
	return &decoder{factory: f}, nil
}

type decoder struct {
	factory *Factory
}

// Decode implements extension.ImageDecoder.
func (d *decoder) Decode(ctx context.Context, src io.ReaderAt, size int64, cfg extension.ImageConfig) (*extension.ImagePlane, error) {
	if d == nil || d.factory == nil {
		return nil, errors.New("openjpeg: nil decoder")
	}
	if src == nil || size <= 0 {
		return nil, errors.New("openjpeg: empty JPX stream")
	}
	dir, err := os.MkdirTemp("", "go-pdf-openjpeg-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	in := filepath.Join(dir, "in.jp2")
	out := filepath.Join(dir, "out.png")
	if err := writeReaderAt(in, src, size); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, d.factory.binary, "-i", in, "-o", out)
	msg, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("openjpeg: decode: %w: %s", err, strings.TrimSpace(string(msg)))
	}
	f, err := os.Open(out)
	if err != nil {
		return nil, err
	}
	img, err := png.Decode(f)
	closeErr := f.Close()
	if err != nil {
		return nil, fmt.Errorf("openjpeg: decode PNG: %w", err)
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return planeFromImage(img)
}

// Close implements extension.ImageDecoder.
func (d *decoder) Close() error { return nil }

func writeReaderAt(path string, r io.ReaderAt, size int64) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, io.NewSectionReader(r, 0, size))
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func planeFromImage(src image.Image) (*extension.ImagePlane, error) {
	if src == nil {
		return nil, errors.New("openjpeg: nil image")
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil, errors.New("openjpeg: empty image")
	}
	if _, ok := src.(*image.Gray); ok {
		g := image.NewGray(image.Rect(0, 0, w, h))
		draw.Draw(g, g.Bounds(), src, b.Min, draw.Src)
		return &extension.ImagePlane{
			Pix: g.Pix, Stride: g.Stride, Width: w, Height: h,
			Components: 1, BitsPerComponent: 8, ColorSpace: "DeviceGray",
		}, nil
	}
	nrgba := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(nrgba, nrgba.Bounds(), src, b.Min, draw.Src)
	opaque := true
	for i := 3; i < len(nrgba.Pix); i += 4 {
		if nrgba.Pix[i] != 0xff {
			opaque = false
			break
		}
	}
	if opaque {
		rgb := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.Draw(rgb, rgb.Bounds(), nrgba, image.Point{}, draw.Src)
		pix := make([]byte, 0, w*h*3)
		for i := 0; i < len(rgb.Pix); i += 4 {
			pix = append(pix, rgb.Pix[i], rgb.Pix[i+1], rgb.Pix[i+2])
		}
		return &extension.ImagePlane{
			Pix: pix, Stride: w * 3, Width: w, Height: h,
			Components: 3, BitsPerComponent: 8, ColorSpace: "DeviceRGB",
		}, nil
	}
	return &extension.ImagePlane{
		Pix: nrgba.Pix, Stride: nrgba.Stride, Width: w, Height: h,
		Components: 4, BitsPerComponent: 8, ColorSpace: "DeviceRGB",
		Alpha: true,
	}, nil
}
