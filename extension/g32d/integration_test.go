package g32d

import (
	"context"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/F31/go-pdf"
	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/extension"
	"github.com/F31/go-pdf/extension/poppler"
	"github.com/F31/go-pdf/internal/testkit"
	"github.com/F31/go-pdf/render"
)

func TestProviderPDFIntegration(t *testing.T) {
	rows := [][]bool{
		{true, false, true, false},
		{false, true, false, true},
	}
	data := encodeRows(4, rows, []bool{true, true})
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 4 2] /Resources << /XObject << /Im1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("q 4 0 0 2 0 0 cm /Im1 Do Q")),
		testkit.Stream(5, "<< /Type /XObject /Subtype /Image /Width 4 /Height 2 /BitsPerComponent 1 /ColorSpace /DeviceGray /Filter /CCITTFaxDecode /DecodeParms << /K 1 /Columns 4 /Rows 2 >> >>", data),
	}
	path := filepath.Join(t.TempDir(), "g32d.pdf")
	if err := os.WriteFile(path, testkit.Build(objs, "/Root 1 0 R"), 0o600); err != nil {
		t.Fatal(err)
	}
	reg, err := extension.NewRegistry(extension.WithImageFactory(New()))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := pdf.OpenFile(context.Background(), path, pdf.WithExtensionRegistry(reg))
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	page, err := doc.Page(0)
	if err != nil {
		t.Fatal(err)
	}
	core, err := render.RenderRegion(context.Background(), page, displaylist.Rect{X1: 4, Y1: 2}, render.PageOptions{
		DPI: 72, Background: "white", MaxPixels: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if core.Bounds().Dx() != 4 || core.Bounds().Dy() != 2 {
		t.Fatalf("core bounds = %v", core.Bounds())
	}
	black := func(img image.Image) int {
		n := 0
		for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
			for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
				r, _, _, _ := img.At(x, y).RGBA()
				if r < 0x8000 {
					n++
				}
			}
		}
		return n
	}
	if got := black(core); got != 4 {
		t.Fatalf("core black pixels = %d, want 4", got)
	}

	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}
	pr, err := poppler.New("")
	if err != nil {
		t.Skipf("poppler unavailable: %v", err)
	}
	pr.DPI = 72
	ref, err := pr.RenderPage(context.Background(), path, 0)
	if err != nil {
		t.Fatalf("poppler: %v", err)
	}
	if ref.Bounds().Dx() != 4 || ref.Bounds().Dy() != 2 {
		t.Fatalf("poppler bounds = %v", ref.Bounds())
	}
	if got := black(ref); got != 4 {
		t.Fatalf("poppler black pixels = %d, want 4", got)
	}
}
