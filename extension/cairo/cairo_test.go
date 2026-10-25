package cairo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/F31/go-pdf/internal/testkit"
)

func TestRenderSVG(t *testing.T) {
	if _, err := exec.LookPath("pdftocairo"); err != nil {
		t.Skip("pdftocairo not installed")
	}
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>"),
	}
	path := filepath.Join(t.TempDir(), "test.pdf")
	if err := os.WriteFile(path, testkit.Build(objs, "/Root 1 0 R"), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := New("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Version(context.Background()); err != nil {
		t.Fatal(err)
	}
	svg, err := r.RenderSVG(context.Background(), path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(svg) == 0 || string(svg[:5]) != "<?xml" {
		t.Fatalf("svg = %q", svg[:min(len(svg), 40)])
	}
	if _, err := r.RenderSVG(context.Background(), path, -1); err == nil {
		t.Fatal("expected invalid page error")
	}
}

func TestProviderMetadataAndBoundedErrors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake pdftocairo requires a POSIX shell")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "pdftocairo")
	body := "#!/bin/sh\nif [ \"$1\" = \"-v\" ]; then echo 'fake cairo 1.0'; exit 0; fi\nhead -c 200000 /dev/zero | tr '\\0' x >&2\nexit 1\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	r, err := New(script)
	if err != nil {
		t.Fatal(err)
	}
	info := r.ProviderInfo(context.Background())
	if info.Name != "poppler-cairo" || info.License == "" || info.Version == "" {
		t.Fatalf("info = %#v", info)
	}
	if _, err := r.RenderSVG(context.Background(), "dummy.pdf", 0); err == nil || len(err.Error()) > 80_000 {
		t.Fatalf("error = %v (%d bytes)", err, len(err.Error()))
	}
}
