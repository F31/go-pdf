package poppler

import (
	"context"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/F31/go-pdf/internal/testkit"
)

func TestRenderPage(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R 4 0 R 5 0 R] /Count 3 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>"),
		testkit.Raw(4, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 60 80] >>"),
		testkit.Raw(5, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 50 40] >>"),
	}
	path := filepath.Join(t.TempDir(), "test.pdf")
	if err := os.WriteFile(path, testkit.Build(objs, "/Root 1 0 R"), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := New("")
	if err != nil {
		t.Fatal(err)
	}
	r.DPI = 72
	if _, err := r.Version(context.Background()); err != nil {
		t.Fatal(err)
	}
	img, err := r.RenderPage(context.Background(), path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 100 || img.Bounds().Dy() != 100 {
		t.Fatalf("bounds = %v", img.Bounds())
	}
	if _, err := r.RenderPage(context.Background(), path, -1); err == nil {
		t.Fatal("expected invalid page error")
	}
	var pages []int
	err = r.RenderEach(context.Background(), path, 0, 1, func(page int, img image.Image) error {
		pages = append(pages, page)
		if page == 1 && (img.Bounds().Dx() != 60 || img.Bounds().Dy() != 80) {
			t.Fatalf("page 1 bounds = %v", img.Bounds())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 || pages[0] != 0 || pages[1] != 1 {
		t.Fatalf("pages = %v", pages)
	}
	if err := r.RenderEach(context.Background(), path, 1, 0, func(int, image.Image) error { return nil }); err == nil {
		t.Fatal("expected invalid range error")
	}
	var subset []int
	if err := r.RenderPages(context.Background(), path, []int{2, 0, 2}, func(page int, img image.Image) error {
		subset = append(subset, page)
		if page == 2 && (img.Bounds().Dx() != 50 || img.Bounds().Dy() != 40) {
			t.Fatalf("page 2 bounds = %v", img.Bounds())
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(subset) != 2 || subset[0] != 0 || subset[1] != 2 {
		t.Fatalf("subset = %v", subset)
	}
}

func TestProviderMetadataAndBoundedErrors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake pdftoppm requires a POSIX shell")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "pdftoppm")
	body := "#!/bin/sh\nif [ \"$1\" = \"-v\" ]; then echo 'fake poppler 1.0'; exit 0; fi\nhead -c 200000 /dev/zero | tr '\\0' x >&2\nexit 1\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	r, err := New(script)
	if err != nil {
		t.Fatal(err)
	}
	info := r.ProviderInfo(context.Background())
	if info.Name != "poppler" || info.License == "" || info.Version == "" {
		t.Fatalf("info = %#v", info)
	}
	if _, err := r.RenderPage(context.Background(), "dummy.pdf", 0); err == nil || len(err.Error()) > 80_000 {
		t.Fatalf("error = %v (%d bytes)", err, len(err.Error()))
	}
}
