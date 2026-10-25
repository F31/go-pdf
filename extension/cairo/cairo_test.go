package cairo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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
