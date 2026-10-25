package jbig2

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/F31/go-pdf/extension"
)

func TestFactoryMetadata(t *testing.T) {
	if _, err := exec.LookPath("jbig2dec"); err != nil {
		t.Skip("jbig2dec not installed")
	}
	f, err := New("")
	if err != nil {
		t.Fatal(err)
	}
	info := f.Info()
	if info.Name != "jbig2dec" || info.Version == "" || info.License == "" {
		t.Fatalf("info = %#v", info)
	}
}

func TestDecodeWithFakeBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake jbig2dec requires a POSIX shell")
	}
	dir := t.TempDir()
	img := image.NewGray(image.Rect(0, 0, 2, 1))
	img.SetGray(0, 0, color.Gray{Y: 10})
	img.SetGray(1, 0, color.Gray{Y: 20})
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, img); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(dir, "fixture.png")
	if err := os.WriteFile(fixture, pngData.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "jbig2dec")
	body := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "-h" ]; then echo "fake jbig2dec 1.0"; exit 0; fi
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
	if f.Info().Version != "fake jbig2dec 1.0" {
		t.Fatalf("version = %q", f.Info().Version)
	}
	dec, err := f.NewDecoder(context.Background(), extension.ImageConfig{})
	if err != nil {
		t.Fatal(err)
	}
	plane, err := dec.Decode(context.Background(), bytes.NewReader([]byte("jb2")), 3, extension.ImageConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if plane.Width != 2 || plane.Height != 1 || plane.Components != 1 {
		t.Fatalf("plane = %#v", plane)
	}
	if plane.Pix[0] != 10 || plane.Pix[1] != 20 {
		t.Fatalf("pix = %v", plane.Pix)
	}
}
