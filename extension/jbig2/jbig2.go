// Package jbig2 provides an optional JBIG2 image provider backed by jbig2dec.
package jbig2

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
	"github.com/F31/go-pdf/internal/procutil"
)

const (
	maxOutputBytes  = 64 << 20
	maxOutputPixels = 64_000_000
)

// Factory invokes jbig2dec. It is immutable and safe for concurrent use.
type Factory struct {
	binary  string
	version string
}

// New locates binary (or jbig2dec when empty) and records its version.
func New(binary string) (*Factory, error) {
	if binary == "" {
		binary = "jbig2dec"
	}
	path, err := exec.LookPath(binary)
	if err != nil {
		return nil, fmt.Errorf("jbig2: executable %q: %w", binary, err)
	}
	f := &Factory{binary: path, version: "unknown"}
	out, _ := procutil.Run(context.Background(), path, "-h")
	if line := procutil.FirstLine(out); line != "" {
		f.version = line
	}
	return f, nil
}

// Info implements extension.Provider.
func (f *Factory) Info() extension.ProviderInfo {
	return extension.ProviderInfo{
		Name: "jbig2dec", Version: f.version, License: "GPL-2.0-or-later",
		Determinism: extension.ProviderDependent,
	}
}

// Features implements extension.Provider.
func (f *Factory) Features() []extension.Feature {
	return []extension.Feature{{
		Name: string(extension.ImageJBIG2), Status: extension.StatusExternal,
		Reason: "decoded by the jbig2dec process",
	}}
}

// Encodings implements extension.ImageDecoderFactory.
func (f *Factory) Encodings() []extension.ImageEncoding {
	return []extension.ImageEncoding{extension.ImageJBIG2}
}

// NewDecoder implements extension.ImageDecoderFactory.
func (f *Factory) NewDecoder(context.Context, extension.ImageConfig) (extension.ImageDecoder, error) {
	if f == nil {
		return nil, errors.New("jbig2: nil factory")
	}
	return &decoder{factory: f}, nil
}

type decoder struct{ factory *Factory }

// Decode implements extension.ImageDecoder.
func (d *decoder) Decode(ctx context.Context, src io.ReaderAt, size int64, cfg extension.ImageConfig) (*extension.ImagePlane, error) {
	if d == nil || d.factory == nil {
		return nil, errors.New("jbig2: nil decoder")
	}
	if src == nil || size <= 0 {
		return nil, errors.New("jbig2: empty JBIG2 stream")
	}
	dir, err := os.MkdirTemp("", "go-pdf-jbig2-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	in := filepath.Join(dir, "in.jb2")
	out := filepath.Join(dir, "out.png")
	if err := writeReaderAt(in, src, size); err != nil {
		return nil, err
	}
	msg, err := procutil.Run(ctx, d.factory.binary, "-o", out, in)
	if err != nil {
		return nil, fmt.Errorf("jbig2: decode: %w: %s", err, strings.TrimSpace(msg))
	}
	if err := procutil.CheckFileSize(out, maxOutputBytes); err != nil {
		return nil, fmt.Errorf("jbig2: output: %w", err)
	}
	f, err := os.Open(out)
	if err != nil {
		return nil, err
	}
	pngCfg, err := png.DecodeConfig(f)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("jbig2: decode PNG config: %w", err)
	}
	if int64(pngCfg.Width)*int64(pngCfg.Height) > maxOutputPixels {
		_ = f.Close()
		return nil, fmt.Errorf("jbig2: output is %dx%d, over pixel limit", pngCfg.Width, pngCfg.Height)
	}
	if _, err := f.Seek(0, 0); err != nil {
		_ = f.Close()
		return nil, err
	}
	img, err := png.Decode(f)
	closeErr := f.Close()
	if err != nil {
		return nil, fmt.Errorf("jbig2: decode PNG: %w", err)
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
		return nil, errors.New("jbig2: nil image")
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil, errors.New("jbig2: empty image")
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
		Components: 4, BitsPerComponent: 8, ColorSpace: "DeviceRGB", Alpha: true,
	}, nil
}
