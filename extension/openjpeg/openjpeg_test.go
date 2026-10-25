package openjpeg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/F31/go-pdf/extension"
)

func TestFactoryMetadata(t *testing.T) {
	if _, err := exec.LookPath("opj_decompress"); err != nil {
		t.Skip("opj_decompress not installed")
	}
	f, err := New("")
	if err != nil {
		t.Fatal(err)
	}
	info := f.Info()
	if info.Name != "openjpeg" || info.Version == "" || info.License == "" {
		t.Fatalf("info = %#v", info)
	}
	encs := f.Encodings()
	if len(encs) != 1 || encs[0] != extension.ImageJPX {
		t.Fatalf("encodings = %#v", encs)
	}
	d, err := f.NewDecoder(context.Background(), extension.ImageConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Decode(context.Background(), nil, 0, extension.ImageConfig{}); err == nil {
		t.Fatal("expected empty stream error")
	}
}

func TestPlaneFromImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.Set(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	img.Set(1, 0, color.RGBA{R: 4, G: 5, B: 6, A: 255})
	p, err := planeFromImage(img)
	if err != nil {
		t.Fatal(err)
	}
	if p.Components != 3 || p.Width != 2 || p.Height != 1 || len(p.Pix) != 6 {
		t.Fatalf("plane = %#v", p)
	}
	if p.Pix[0] != 1 || p.Pix[5] != 6 {
		t.Fatalf("pix = %v", p.Pix)
	}
}

func TestDecodeWithFakeBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake opj_decompress requires a POSIX shell")
	}
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.Set(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	img.Set(1, 0, color.RGBA{R: 40, G: 50, B: 60, A: 255})
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, img); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(dir, "fixture.png")
	if err := os.WriteFile(fixture, pngData.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "opj_decompress")
	body := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "-h" ]; then echo "fake openjpeg 1.0"; exit 0; fi
out=""
prev=""
for arg in "$@"; do
  if [ "$prev" = "-o" ]; then out="$arg"; fi
  prev="$arg"
done
if [ -z "$out" ]; then exit 2; fi
cp %q "$out"
`, fixture)
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := New(script)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Info().Version; got != "fake openjpeg 1.0" {
		t.Fatalf("version = %q", got)
	}
	dec, err := f.NewDecoder(context.Background(), extension.ImageConfig{})
	if err != nil {
		t.Fatal(err)
	}
	plane, err := dec.Decode(context.Background(), bytes.NewReader([]byte("jp2")), 3, extension.ImageConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if plane.Width != 2 || plane.Height != 1 || plane.Components != 3 {
		t.Fatalf("plane = %#v", plane)
	}
	if plane.Pix[0] != 10 || plane.Pix[2] != 30 || plane.Pix[3] != 40 || plane.Pix[5] != 60 {
		t.Fatalf("pix = %v", plane.Pix)
	}
}

func TestDecodeContextCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake opj_decompress requires a POSIX shell")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "opj_decompress")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nif [ \"$1\" = \"-h\" ]; then echo fake; exit 0; fi\nsleep 0.2\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := New(script)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := f.NewDecoder(context.Background(), extension.ImageConfig{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := dec.Decode(ctx, bytes.NewReader([]byte("jp2")), 3, extension.ImageConfig{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
}

func TestDecodeErrorMessageIsBounded(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake opj_decompress requires a POSIX shell")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "opj_decompress")
	body := "#!/bin/sh\nif [ \"$1\" = \"-h\" ]; then echo fake; exit 0; fi\nhead -c 200000 /dev/zero | tr '\\0' x >&2\nexit 1\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := New(script)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := f.NewDecoder(context.Background(), extension.ImageConfig{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = dec.Decode(context.Background(), bytes.NewReader([]byte("jp2")), 3, extension.ImageConfig{})
	if err == nil {
		t.Fatal("expected decode error")
	}
	if len(err.Error()) > 70_000 || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("error is not bounded: %d bytes", len(err.Error()))
	}
}
