package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/F31/go-pdf/internal/testkit"
)

func writePDF(t *testing.T) string {
	t.Helper()
	data := testkit.Build([]testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /Font << /F1 6 0 R >> >> /Annots [5 0 R] /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("BT /F1 12 Tf 10 50 Td (Hi) Tj ET")),
		testkit.Raw(5, "<< /Type /Annot /Subtype /Link /Rect [10 10 50 30] /A << /S /URI /URI (https://example.test/) >> >>"),
		testkit.Raw(6, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"),
	}, "/Root 1 0 R")
	path := filepath.Join(t.TempDir(), "test.pdf")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCLIInfoTextRenderLinks(t *testing.T) {
	path := writePDF(t)
	var out, errOut bytes.Buffer
	if err := run([]string{"info", path}, &out, &errOut); err != nil {
		t.Fatal(err, errOut.String())
	}
	if !strings.Contains(out.String(), "Pages: 1") {
		t.Fatalf("info %q", out.String())
	}
	out.Reset()
	if err := run([]string{"text", path}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Hi") {
		t.Fatalf("text %q", out.String())
	}
	out.Reset()
	if err := run([]string{"search", "hi", path}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Hi") {
		t.Fatalf("search %q", out.String())
	}
	out.Reset()
	if err := run([]string{"preflight", path}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "render\tapproximate/success") {
		t.Fatalf("preflight %q", out.String())
	}
	out.Reset()
	if err := run([]string{"links", path}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "https://example.test/") {
		t.Fatalf("links %q", out.String())
	}
	var capOut bytes.Buffer
	if err := run([]string{"capabilities"}, &capOut, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(capOut.String(), "security") {
		t.Fatalf("capabilities %q", capOut.String())
	}
	pngPath := filepath.Join(t.TempDir(), "page.png")
	if err := run([]string{"render", "-o", pngPath, "-dpi", "72", path}, &out, &errOut); err != nil {
		t.Fatal(err, errOut.String())
	}
	f, err := os.Open(pngPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil || img.Bounds().Dx() != 100 {
		t.Fatalf("png %v %v", img, err)
	}
}

func TestCLIEncryptedPassword(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "encrypted", "rc4-128.pdf")
	var out, errOut bytes.Buffer
	if err := run([]string{"text", "-password", "user", path}, &out, &errOut); err != nil {
		t.Fatal(err, errOut.String())
	}
	if !strings.Contains(out.String(), "Hello encrypted") {
		t.Fatalf("text %q", out.String())
	}
	out.Reset()
	if err := run([]string{"text", path}, &out, &errOut); err == nil {
		t.Fatal("expected password error")
	}
}

func TestCLICompare(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	dir := t.TempDir()
	a := filepath.Join(dir, "a.png")
	b := filepath.Join(dir, "b.png")
	for _, path := range []string{a, b} {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	var out, errOut bytes.Buffer
	if err := run([]string{"compare", "-json", a, b}, &out, &errOut); err != nil {
		t.Fatal(err, errOut.String())
	}
	if !strings.Contains(out.String(), `"SSIM":1`) {
		t.Fatalf("compare %q", out.String())
	}
}
