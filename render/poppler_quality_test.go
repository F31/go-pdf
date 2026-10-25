package render

import (
	"bytes"
	"context"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/F31/go-pdf"
	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/internal/quality"
	"github.com/F31/go-pdf/internal/testkit"
)

func TestPopplerReferenceQuality(t *testing.T) {
	if os.Getenv("POPPLER_QUALITY") == "" {
		t.Skip("set POPPLER_QUALITY=1 to compare against pdftoppm")
	}
	bin, err := exec.LookPath("pdftoppm")
	if err != nil {
		t.Skip("pdftoppm not installed")
	}
	base := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
	}
	cases := []struct {
		name string
		objs []testkit.Obj
	}{
		{
			name: "solid-fill",
			objs: append(append([]testkit.Obj{}, base...),
				testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Contents 4 0 R >>"),
				testkit.Stream(4, "", []byte("1 0 0 rg 10 10 80 80 re f 0 0 1 rg 30 30 40 40 re f")),
			),
		},
		{
			name: "constant-alpha",
			objs: append(append([]testkit.Obj{}, base...),
				testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /ExtGState << /GS1 5 0 R >> >> /Contents 4 0 R >>"),
				testkit.Stream(4, "", []byte("/GS1 gs 1 0 0 rg 10 10 80 80 re f")),
				testkit.Raw(5, "<< /Type /ExtGState /ca 0.5 >>"),
			),
		},
		{
			name: "luminosity-soft-mask",
			objs: append(append([]testkit.Obj{}, base...),
				testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /ExtGState << /GS1 5 0 R >> >> /Contents 4 0 R >>"),
				testkit.Stream(4, "", []byte("/GS1 gs 1 0 0 rg 10 10 80 80 re f")),
				testkit.Raw(5, "<< /Type /ExtGState /SMask 6 0 R >>"),
				testkit.Raw(6, "<< /S /Luminosity /G 7 0 R >>"),
				testkit.Stream(7, "<< /Type /XObject /Subtype /Form /BBox [0 0 100 100] /Group << /S /Transparency /CS /DeviceGray >> >>", []byte("0.5 g 0 0 100 100 re f")),
			),
		},
	}
	// Knockout groups are intentionally excluded: the local pdftoppm/Splash
	// build did not apply knockout semantics to this fixture, so it is not a
	// valid oracle. Those cases are covered analytically in quality_gate_test.go.
	dir := t.TempDir()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := testkit.Build(tc.objs, "/Root 1 0 R")
			pdfPath := filepath.Join(dir, tc.name+".pdf")
			if err := os.WriteFile(pdfPath, data, 0o600); err != nil {
				t.Fatal(err)
			}
			d, err := pdf.Open(context.Background(), bytes.NewReader(data), int64(len(data)))
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			p, err := d.Page(0)
			if err != nil {
				t.Fatal(err)
			}
			got, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white", Fonts: BuiltinFonts()})
			if err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(dir, tc.name)
			cmd := exec.Command(bin, "-f", "1", "-l", "1", "-r", "72", "-png", "-singlefile", pdfPath, out)
			if b, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("pdftoppm: %v: %s", err, b)
			}
			f, err := os.Open(out + ".png")
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			ref, err := png.Decode(f)
			if err != nil {
				t.Fatal(err)
			}
			report, err := quality.Compare(got, ref)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("poppler: %+v", report)
			if err := (quality.Thresholds{MinPSNR: 40, MinSSIM: 0.99, MaxMeanDeltaE: 1, MaxP95DeltaE: 2}).Check(report); err != nil {
				t.Fatal(err)
			}
		})
	}
}
