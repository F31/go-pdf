package render

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"testing"

	"github.com/F31/go-pdf"
	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/internal/quality"
	"github.com/F31/go-pdf/internal/testkit"
)

func uniformRGBA(bounds image.Rectangle, c color.RGBA) *image.RGBA {
	out := image.NewRGBA(bounds)
	draw.Draw(out, out.Bounds(), image.NewUniform(c), image.Point{}, draw.Src)
	return out
}

func cropRGBA(src *image.RGBA, r image.Rectangle) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(out, out.Bounds(), src, r.Min, draw.Src)
	return out
}

func checkGate(t *testing.T, name string, got, want *image.RGBA) {
	t.Helper()
	report, err := quality.Compare(got, want)
	if err != nil {
		t.Fatal(err)
	}
	if err := (quality.Thresholds{MinPSNR: 45, MinSSIM: 0.999}).Check(report); err != nil {
		t.Fatalf("%s: %v (report %+v)", name, err, report)
	}
}

func TestSoftMaskLuminosityQualityGate(t *testing.T) {
	objs := pageObjs("/GS1 gs 1 0 0 rg 0 0 64 64 re f",
		testkit.Raw(5, "<< /Type /ExtGState /SMask 6 0 R >>"),
		testkit.Raw(6, "<< /S /Luminosity /G 7 0 R >>"),
		testkit.Stream(7, "<< /Type /XObject /Subtype /Form /BBox [0 0 64 64] /Group << /S /Transparency /CS /DeviceGray >> >>", []byte("0.5 g 0 0 64 64 re f")),
	)
	objs[2] = testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 64 64] /Resources << /ExtGState << /GS1 5 0 R >> >> /Contents 4 0 R >>")
	p := openPage(t, objs)
	got, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 64, Y1: 64}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	// Luminosity 0.5 is quantized to 127/255 by the 8-bit soft mask.
	want := uniformRGBA(image.Rect(0, 0, 64, 64), color.RGBA{R: 255, G: 128, B: 128, A: 255})
	checkGate(t, "soft-mask-luminosity", got.(*image.RGBA), want)
}

func TestKnockoutNonIsolatedQualityGate(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /XObject << /Fm1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/Fm1 Do")),
		testkit.Stream(5, "<< /Type /XObject /Subtype /Form /BBox [0 0 100 100] /Group << /S /Transparency /K true >> /Resources << /ExtGState << /GS1 6 0 R >> >> >>", []byte("q /GS1 gs 1 0 0 rg 0 0 50 100 re f Q q /GS1 gs 0 0 1 rg 25 0 50 100 re f Q")),
		testkit.Raw(6, "<< /Type /ExtGState /ca 0.5 >>"),
	}
	data := testkit.Build(objs, "/Root 1 0 R")
	d, err := pdf.Open(context.Background(), bytes.NewReader(data), int64(len(data)), pdf.WithPreserveForms(true))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	p, err := d.Page(0)
	if err != nil {
		t.Fatal(err)
	}
	got, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	rgba := got.(*image.RGBA)
	checkGate(t, "knockout-red", cropRGBA(rgba, image.Rect(10, 40, 20, 60)), uniformRGBA(image.Rect(0, 0, 10, 20), color.RGBA{R: 255, G: 128, B: 128, A: 255}))
	checkGate(t, "knockout-overlap", cropRGBA(rgba, image.Rect(35, 40, 45, 60)), uniformRGBA(image.Rect(0, 0, 10, 20), color.RGBA{R: 128, G: 128, B: 255, A: 255}))
	checkGate(t, "knockout-blue", cropRGBA(rgba, image.Rect(60, 40, 70, 60)), uniformRGBA(image.Rect(0, 0, 10, 20), color.RGBA{R: 128, G: 128, B: 255, A: 255}))
}

func TestSoftMaskAlphaQualityGate(t *testing.T) {
	objs := pageObjs("/GS1 gs 1 0 0 rg 0 0 64 64 re f",
		testkit.Raw(5, "<< /Type /ExtGState /SMask 6 0 R >>"),
		testkit.Raw(6, "<< /S /Alpha /G 7 0 R >>"),
		testkit.Stream(7, "<< /Type /XObject /Subtype /Form /BBox [0 0 64 64] /Group << /S /Transparency /CS /DeviceGray >> /Resources << /ExtGState << /GS2 8 0 R >> >> >>", []byte("/GS2 gs 0.5 g 0 0 64 64 re f")),
		testkit.Raw(8, "<< /Type /ExtGState /ca 0.5 >>"),
	)
	objs[2] = testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 64 64] /Resources << /ExtGState << /GS1 5 0 R >> >> /Contents 4 0 R >>")
	p := openPage(t, objs)
	got, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 64, Y1: 64}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	want := uniformRGBA(image.Rect(0, 0, 64, 64), color.RGBA{R: 255, G: 128, B: 128, A: 255})
	checkGate(t, "soft-mask-alpha", got.(*image.RGBA), want)
}
