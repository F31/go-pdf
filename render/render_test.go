package render

import (
	"bytes"
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"os"
	"sync"
	"testing"

	"github.com/F31/go-pdf"
	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/extension"
	"github.com/F31/go-pdf/internal/testkit"
)

func openPage(t *testing.T, objs []testkit.Obj) *pdf.Page {
	t.Helper()
	data := testkit.Build(objs, "/Root 1 0 R")
	d, err := pdf.Open(context.Background(), bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	p, err := d.Page(0)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func pageObjs(content string, extra ...testkit.Obj) []testkit.Obj {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte(content)),
	}
	return append(objs, extra...)
}

func TestRenderPathAndRegion(t *testing.T) {
	p := openPage(t, pageObjs("1 0 0 rg 0 0 100 100 re f"))
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 100 || img.Bounds().Dy() != 100 {
		t.Fatalf("size %v", img.Bounds())
	}
	if r, g, b, _ := img.At(50, 50).RGBA(); r>>8 != 255 || g>>8 != 0 || b>>8 != 0 {
		t.Fatalf("pixel %v", img.At(50, 50))
	}
	img, err = RenderRegion(context.Background(), p, displaylist.Rect{X0: 25, Y0: 25, X1: 75, Y1: 75}, PageOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 50 || img.At(1, 1).(color.RGBA).R != 255 {
		t.Fatalf("region %v %v", img.Bounds(), img.At(1, 1))
	}
}

func TestRenderClip(t *testing.T) {
	p := openPage(t, pageObjs("q 0 0 50 100 re W n 1 0 0 rg 0 0 100 100 re f Q"))
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	if r, _, _, _ := img.At(10, 50).RGBA(); r>>8 != 255 {
		t.Fatalf("inside clip %v", img.At(10, 50))
	}
	if r, g, b, _ := img.At(70, 50).RGBA(); r>>8 != 255 || g>>8 != 255 || b>>8 != 255 {
		t.Fatalf("outside clip %v", img.At(70, 50))
	}
}

func TestRenderImage(t *testing.T) {
	objs := pageObjs("q 100 0 0 100 0 0 cm /Im1 Do Q",
		testkit.Stream(5, "<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /BitsPerComponent 8 /ColorSpace /DeviceRGB >>", []byte{255, 0, 0}),
	)
	// Replace the page resources with an XObject entry.
	objs[2] = testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /XObject << /Im1 5 0 R >> >> /Contents 4 0 R >>")
	p := openPage(t, objs)
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, _ := img.At(50, 50).RGBA(); r>>8 != 255 || g>>8 != 0 || b>>8 != 0 {
		t.Fatalf("image pixel %v", img.At(50, 50))
	}
}

func TestRenderTextOutline(t *testing.T) {
	fontData, err := os.ReadFile("../testdata/fonts/gopdf-test-sans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	objs := pageObjs("BT /F1 50 Tf 10 20 Td (H) Tj ET",
		testkit.Raw(5, "<< /Type /Font /Subtype /TrueType /BaseFont /GoPDFTestSans /Encoding /WinAnsiEncoding /FontDescriptor 6 0 R >>"),
		testkit.Raw(6, "<< /Type /FontDescriptor /FontName /GoPDFTestSans /Flags 32 /FontBBox [0 0 1000 1000] /ItalicAngle 0 /Ascent 905 /Descent -212 /CapHeight 700 /StemV 80 /FontFile2 7 0 R >>"),
		testkit.Stream(7, "", fontData),
	)
	objs[2] = testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>")
	p := openPage(t, objs)
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white", IncludeText: true})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			if r, g, b, _ := img.At(x, y).RGBA(); r>>8 < 200 || g>>8 < 200 || b>>8 < 200 {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("text outline produced no dark pixels")
	}
}

func TestRenderStroke(t *testing.T) {
	p := openPage(t, pageObjs("4 w 0 50 m 100 50 l S"))
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, _ := img.At(50, 50).RGBA(); r>>8 > 80 || g>>8 > 80 || b>>8 > 80 {
		t.Fatalf("stroke pixel %v", img.At(50, 50))
	}
}

func TestRenderShading(t *testing.T) {
	objs := pageObjs("/Sh1 sh",
		testkit.Raw(6, "<< /ShadingType 2 /ColorSpace /DeviceRGB /Coords [0 0 100 0] /Function 7 0 R /Extend [true true] >>"),
		testkit.Raw(7, "<< /FunctionType 2 /Domain [0 1] /C0 [1 0 0] /C1 [0 0 1] /N 1 >>"),
	)
	objs[2] = testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /Shading << /Sh1 6 0 R >> >> /Contents 4 0 R >>")
	p := openPage(t, objs)
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	if r, _, b, _ := img.At(5, 50).RGBA(); r>>8 < 200 || b>>8 > 60 {
		t.Fatalf("left shading %v", img.At(5, 50))
	}
	if r, _, b, _ := img.At(95, 50).RGBA(); r>>8 > 60 || b>>8 < 200 {
		t.Fatalf("right shading %v", img.At(95, 50))
	}
}

func TestRenderUnsupportedImage(t *testing.T) {
	objs := pageObjs("/Im1 Do",
		testkit.Stream(5, "<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /BitsPerComponent 1 /ColorSpace /DeviceGray /Filter /JBIG2Decode >>", []byte{1}),
	)
	objs[2] = testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /XObject << /Im1 5 0 R >> >> /Contents 4 0 R >>")
	p := openPage(t, objs)
	_, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72})
	if err == nil || !errors.Is(err, pdf.ErrUnsupported) {
		t.Fatalf("error %v", err)
	}
}

func TestRenderExcludeAndBudget(t *testing.T) {
	p := openPage(t, pageObjs("1 0 0 rg 0 0 100 100 re f"))
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{
		DPI: 72, Background: "white",
		Exclude: func(it displaylist.Item) bool { _, ok := it.(*displaylist.Path); return ok },
	})
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, _ := img.At(50, 50).RGBA(); r>>8 != 255 || g>>8 != 255 || b>>8 != 255 {
		t.Fatalf("excluded pixel %v", img.At(50, 50))
	}
	if _, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, MaxPixels: 10}); !errors.Is(err, pdf.ErrBudget) {
		t.Fatalf("budget error %v", err)
	}
}

func TestRenderBlendMode(t *testing.T) {
	objs := pageObjs("1 0 0 rg 0 0 100 100 re f q /GS1 gs 0 0 1 rg 0 0 100 100 re f Q",
		testkit.Raw(5, "<< /Type /ExtGState /BM /Multiply >>"),
	)
	objs[2] = testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /ExtGState << /GS1 5 0 R >> >> /Contents 4 0 R >>")
	p := openPage(t, objs)
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, _ := img.At(50, 50).RGBA(); r>>8 > 20 || g>>8 > 20 || b>>8 > 20 {
		t.Fatalf("multiply pixel %v", img.At(50, 50))
	}
}

func TestRenderColorKeyMask(t *testing.T) {
	objs := pageObjs("q 100 0 0 100 0 0 cm /Im1 Do Q",
		testkit.Stream(5, "<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /BitsPerComponent 8 /ColorSpace /DeviceRGB /Mask [1 1 0 0 0 0] >>", []byte{255, 0, 0}),
	)
	objs[2] = testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /XObject << /Im1 5 0 R >> >> /Contents 4 0 R >>")
	p := openPage(t, objs)
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, _ := img.At(50, 50).RGBA(); r>>8 != 255 || g>>8 != 255 || b>>8 != 255 {
		t.Fatalf("masked pixel %v", img.At(50, 50))
	}
}

func TestRenderEvenOdd(t *testing.T) {
	p := openPage(t, pageObjs("1 0 0 rg 0 0 100 100 re 30 30 40 40 re f*"))
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	if r, _, _, _ := img.At(10, 50).RGBA(); r>>8 != 255 {
		t.Fatalf("ring %v", img.At(10, 50))
	}
	if r, g, b, _ := img.At(50, 50).RGBA(); r>>8 != 255 || g>>8 != 255 || b>>8 != 255 {
		t.Fatalf("hole %v", img.At(50, 50))
	}
}

func TestRenderDash(t *testing.T) {
	p := openPage(t, pageObjs("4 w [10 10] 0 d 0 50 m 100 50 l S"))
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	if r, _, _, _ := img.At(5, 50).RGBA(); r>>8 > 80 {
		t.Fatalf("dash on %v", img.At(5, 50))
	}
	if r, g, b, _ := img.At(15, 50).RGBA(); r>>8 != 255 || g>>8 != 255 || b>>8 != 255 {
		t.Fatalf("dash off %v", img.At(15, 50))
	}
}

func TestRenderImageMask(t *testing.T) {
	objs := pageObjs("1 0 0 rg q 100 0 0 100 0 0 cm /Im1 Do Q",
		testkit.Stream(5, "<< /Type /XObject /Subtype /Image /Width 2 /Height 1 /ImageMask true >>", []byte{0x40}),
	)
	objs[2] = testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /XObject << /Im1 5 0 R >> >> /Contents 4 0 R >>")
	p := openPage(t, objs)
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, _ := img.At(25, 50).RGBA(); r>>8 != 255 || g>>8 != 0 || b>>8 != 0 {
		t.Fatalf("mask paint %v", img.At(25, 50))
	}
	if r, g, b, _ := img.At(75, 50).RGBA(); r>>8 != 255 || g>>8 != 255 || b>>8 != 255 {
		t.Fatalf("mask clear %v", img.At(75, 50))
	}
}

func TestRenderClipEvenOdd(t *testing.T) {
	p := openPage(t, pageObjs("q 0 0 100 100 re 30 30 40 40 re W* n 1 0 0 rg 0 0 100 100 re f Q"))
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	if r, _, _, _ := img.At(10, 50).RGBA(); r>>8 != 255 {
		t.Fatalf("clip ring %v", img.At(10, 50))
	}
	if r, g, b, _ := img.At(50, 50).RGBA(); r>>8 != 255 || g>>8 != 255 || b>>8 != 255 {
		t.Fatalf("clip hole %v", img.At(50, 50))
	}
}

func TestRenderTilingPattern(t *testing.T) {
	objs := pageObjs("/Pattern cs /P1 scn 0 0 100 100 re f",
		testkit.Stream(6, "<< /Type /Pattern /PatternType 1 /PaintType 1 /TilingType 1 /BBox [0 0 10 10] /XStep 10 /YStep 10 /Resources << >> >>", []byte("1 0 0 rg 0 0 5 5 re f")),
	)
	objs[2] = testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /Pattern << /P1 6 0 R >> >> /Contents 4 0 R >>")
	p := openPage(t, objs)
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, _ := img.At(2, 98).RGBA(); r>>8 != 255 || g>>8 != 0 || b>>8 != 0 {
		t.Fatalf("tile paint %v", img.At(2, 98))
	}
	if r, g, b, _ := img.At(8, 98).RGBA(); r>>8 != 255 || g>>8 != 255 || b>>8 != 255 {
		t.Fatalf("tile gap %v", img.At(8, 98))
	}
}

func TestRenderUncoloredTilingPattern(t *testing.T) {
	objs := pageObjs("/PCS cs 1 0 0 /P1 scn 0 0 100 100 re f",
		testkit.Raw(5, "<< /Pattern << /P1 6 0 R >> /ColorSpace << /PCS [/Pattern /DeviceRGB] >> >>"),
		testkit.Stream(6, "<< /Type /Pattern /PatternType 1 /PaintType 2 /TilingType 1 /BBox [0 0 10 10] /XStep 10 /YStep 10 /Resources << >> >>", []byte("0 0 5 5 re f")),
	)
	objs[2] = testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /Pattern << /P1 6 0 R >> /ColorSpace << /PCS [/Pattern /DeviceRGB] >> >> /Contents 4 0 R >>")
	p := openPage(t, objs)
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, _ := img.At(2, 98).RGBA(); r>>8 != 255 || g>>8 != 0 || b>>8 != 0 {
		t.Fatalf("uncolored tile paint %v", img.At(2, 98))
	}
}

func TestRenderMeshShading(t *testing.T) {
	raw := []byte{
		0, 0, 255, 0, 0,
		255, 0, 0, 255, 0,
		0, 255, 0, 0, 255,
		255, 255, 255, 255, 255,
	}
	objs := pageObjs("/Sh1 sh",
		testkit.Stream(6, "<< /ShadingType 5 /ColorSpace /DeviceRGB /BitsPerCoordinate 8 /BitsPerComponent 8 /Decode [0 100 0 100 0 1 0 1 0 1] /VerticesPerRow 2 >>", raw),
	)
	objs[2] = testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /Shading << /Sh1 6 0 R >> >> /Contents 4 0 R >>")
	p := openPage(t, objs)
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, _ := img.At(5, 95).RGBA(); r>>8 < 180 || g>>8 > 80 || b>>8 > 80 {
		t.Fatalf("mesh bottom-left %v", img.At(5, 95))
	}
	if r, g, b, _ := img.At(5, 5).RGBA(); r>>8 > 80 || g>>8 > 80 || b>>8 < 180 {
		t.Fatalf("mesh top-left %v", img.At(5, 5))
	}
}

func TestRenderFreeFormMesh(t *testing.T) {
	raw := []byte{
		0, 0, 0, 255, 0, 0,
		0, 255, 0, 0, 255, 0,
		0, 0, 255, 0, 0, 255,
	}
	objs := pageObjs("/Sh1 sh",
		testkit.Stream(6, "<< /ShadingType 4 /ColorSpace /DeviceRGB /BitsPerCoordinate 8 /BitsPerComponent 8 /BitsPerFlag 8 /Decode [0 100 0 100 0 1 0 1 0 1] >>", raw),
	)
	objs[2] = testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /Shading << /Sh1 6 0 R >> >> /Contents 4 0 R >>")
	p := openPage(t, objs)
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, _ := img.At(10, 90).RGBA(); r>>8 == 255 && g>>8 == 255 && b>>8 == 255 {
		t.Fatalf("mesh triangle missing at %v", img.At(10, 90))
	}
}

func TestRenderFunctionShading(t *testing.T) {
	objs := pageObjs("/Sh1 sh",
		testkit.Raw(6, "<< /ShadingType 1 /ColorSpace /DeviceRGB /Domain [0 1 0 1] /Matrix [100 0 0 100 0 0] /Function 7 0 R >>"),
		testkit.Stream(7, "<< /FunctionType 4 /Domain [0 1 0 1] /Range [0 1 0 1 0 1] >>", []byte("0")),
	)
	objs[2] = testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /Shading << /Sh1 6 0 R >> >> /Contents 4 0 R >>")
	p := openPage(t, objs)
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, _ := img.At(5, 95).RGBA(); r>>8 > 80 || g>>8 > 80 || b>>8 > 80 {
		t.Fatalf("function shading bottom-left %v", img.At(5, 95))
	}
	if r, g, b, _ := img.At(5, 5).RGBA(); r>>8 > 80 || g>>8 < 180 || b>>8 > 80 {
		t.Fatalf("function shading top-left %v", img.At(5, 5))
	}
}

func TestRenderPatchMesh(t *testing.T) {
	raw := []byte{
		0,
		0, 0, 0, 84, 0, 168, 0, 255,
		84, 255, 168, 255, 255, 255, 255, 168,
		255, 84, 255, 0, 168, 0, 84, 0,
		255, 0, 0, 0, 0, 255, 0, 255, 0, 255, 255, 255,
	}
	objs := pageObjs("/Sh1 sh",
		testkit.Stream(6, "<< /ShadingType 6 /ColorSpace /DeviceRGB /BitsPerCoordinate 8 /BitsPerComponent 8 /BitsPerFlag 8 /Decode [0 100 0 100 0 1 0 1 0 1] >>", raw),
	)
	objs[2] = testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /Shading << /Sh1 6 0 R >> >> /Contents 4 0 R >>")
	p := openPage(t, objs)
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, _ := img.At(50, 50).RGBA(); r>>8 == 255 && g>>8 == 255 && b>>8 == 255 {
		t.Fatalf("patch mesh not rendered at %v", img.At(50, 50))
	}
}

func TestRenderSoftMask(t *testing.T) {
	objs := pageObjs("/GS1 gs 1 0 0 rg 0 0 100 100 re f",
		testkit.Raw(5, "<< /Type /ExtGState /SMask 6 0 R >>"),
		testkit.Stream(6, "<< /Type /XObject /Subtype /Form /BBox [0 0 100 100] /Group << /S /Transparency /CS /DeviceGray >> >>", []byte("1 1 1 rg 0 0 50 100 re f")),
	)
	objs[2] = testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /ExtGState << /GS1 5 0 R >> >> /Contents 4 0 R >>")
	p := openPage(t, objs)
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, _ := img.At(10, 50).RGBA(); r>>8 != 255 || g>>8 != 0 || b>>8 != 0 {
		t.Fatalf("soft mask inside %v", img.At(10, 50))
	}
	if r, g, b, _ := img.At(90, 50).RGBA(); r>>8 != 255 || g>>8 != 255 || b>>8 != 255 {
		t.Fatalf("soft mask outside %v", img.At(90, 50))
	}
}

func TestRenderTransparencyGroup(t *testing.T) {
	objs := pageObjs("/Fm1 Do",
		testkit.Stream(5, "<< /Type /XObject /Subtype /Form /BBox [0 0 100 100] /Group << /S /Transparency >> >>", []byte("1 0 0 rg 0 0 100 100 re f")),
	)
	objs[2] = testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /XObject << /Fm1 5 0 R >> >> /Contents 4 0 R >>")
	data := testkit.Build(objs, "/Root 1 0 R")
	d, err := pdf.Open(context.Background(), bytes.NewReader(data), int64(len(data)), pdf.WithPreserveForms(true))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	p, _ := d.Page(0)
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, _ := img.At(50, 50).RGBA(); r>>8 != 255 || g>>8 != 0 || b>>8 != 0 {
		t.Fatalf("group pixel %v", img.At(50, 50))
	}
}

type fakeGlyphs struct{}

func (fakeGlyphs) GlyphOutline(code, cid uint32) ([]displaylist.Segment, bool) {
	return []displaylist.Segment{
		{Kind: displaylist.SegmentMove, P0: displaylist.Point{X: 0, Y: 0}},
		{Kind: displaylist.SegmentLine, P0: displaylist.Point{X: 0, Y: 0}, P3: displaylist.Point{X: 500, Y: 0}},
		{Kind: displaylist.SegmentLine, P0: displaylist.Point{X: 500, Y: 0}, P3: displaylist.Point{X: 500, Y: 700}},
		{Kind: displaylist.SegmentLine, P0: displaylist.Point{X: 500, Y: 700}, P3: displaylist.Point{X: 0, Y: 700}},
		{Kind: displaylist.SegmentClose, P0: displaylist.Point{X: 0, Y: 700}, P3: displaylist.Point{X: 0, Y: 0}},
	}, true
}

type rectangleFontProvider struct{}

func (rectangleFontProvider) GlyphOutline(info *displaylist.FontInfo, glyph displaylist.Glyph) ([]displaylist.Segment, bool, error) {
	return []displaylist.Segment{
		{Kind: displaylist.SegmentMove, P0: displaylist.Point{X: 0, Y: 0}},
		{Kind: displaylist.SegmentLine, P0: displaylist.Point{X: 0, Y: 0}, P3: displaylist.Point{X: 500, Y: 0}},
		{Kind: displaylist.SegmentLine, P0: displaylist.Point{X: 500, Y: 0}, P3: displaylist.Point{X: 500, Y: 700}},
		{Kind: displaylist.SegmentLine, P0: displaylist.Point{X: 500, Y: 700}, P3: displaylist.Point{X: 0, Y: 700}},
		{Kind: displaylist.SegmentClose, P0: displaylist.Point{X: 0, Y: 700}, P3: displaylist.Point{X: 0, Y: 0}},
	}, true, nil
}

func TestRenderFontProvider(t *testing.T) {
	objs := pageObjs("BT /F1 50 Tf 10 20 Td (Hi) Tj ET")
	objs[2] = testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>")
	objs = append(objs, testkit.Raw(5, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"))
	p := openPage(t, objs)
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{
		DPI: 72, Background: "white", IncludeText: true, Fonts: rectangleFontProvider{},
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			if r, g, b, _ := img.At(x, y).RGBA(); r>>8 < 100 && g>>8 < 100 && b>>8 < 100 {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("font provider produced no outlines")
	}
}

func TestRenderAnnotationAppearance(t *testing.T) {
	objs := pageObjs("",
		testkit.Raw(5, "<< /Type /Annot /Subtype /Widget /Rect [20 20 60 60] /P 3 0 R /AP << /N 6 0 R >> >>"),
		testkit.Stream(6, "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Resources << >> >>", []byte("1 0 0 rg 0 0 10 10 re f")),
	)
	objs[2] = testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Annots [5 0 R] /Contents 4 0 R >>")
	p := openPage(t, objs)
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white", Annotations: AnnotationView})
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, _ := img.At(40, 40).RGBA(); r>>8 != 255 || g>>8 != 0 || b>>8 != 0 {
		t.Fatalf("annotation pixel %v", img.At(40, 40))
	}
	img, err = RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, _ := img.At(40, 40).RGBA(); r>>8 != 255 || g>>8 != 255 || b>>8 != 255 {
		t.Fatalf("annotation hidden pixel %v", img.At(40, 40))
	}
}

func TestRenderPageAndEncodePNG(t *testing.T) {
	p := openPage(t, pageObjs("1 0 0 rg 0 0 100 100 re f"))
	img, err := RenderPage(context.Background(), p, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := EncodePNG(&buf, img); err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds().Dx() != 100 || decoded.Bounds().Dy() != 100 {
		t.Fatalf("png bounds %v", decoded.Bounds())
	}
	if r, g, b, _ := decoded.At(50, 50).RGBA(); r>>8 != 255 || g>>8 != 0 || b>>8 != 0 {
		t.Fatalf("png pixel %v", decoded.At(50, 50))
	}
}

func TestRenderJPEG(t *testing.T) {
	p := openPage(t, pageObjs("1 0 0 rg 0 0 100 100 re f"))
	img, err := RenderPage(context.Background(), p, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := EncodeJPEG(&buf, img, 90); err != nil {
		t.Fatal(err)
	}
	decoded, err := jpeg.Decode(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds().Dx() != 100 {
		t.Fatalf("jpeg bounds %v", decoded.Bounds())
	}
}

func TestAnnotationVisibility(t *testing.T) {
	tests := []struct {
		flags int
		mode  AnnotationMode
		want  bool
	}{
		{0, AnnotationView, true},
		{2, AnnotationView, false},
		{1, AnnotationView, false},
		{4, AnnotationView, true},
		{32, AnnotationView, false},
		{36, AnnotationView, false},
	}
	for _, tt := range tests {
		if got := annotationVisible(displaylist.Annotation{Flags: tt.flags}, tt.mode); got != tt.want {
			t.Fatalf("flags=%d mode=%d got=%v want=%v", tt.flags, tt.mode, got, tt.want)
		}
	}
}

type testMemory struct {
	limit int64
	used  int64
}

func (m *testMemory) AcquireMemory(n int64) (func(), error) {
	if m.used+n > m.limit {
		return nil, pdf.ErrBudget
	}
	m.used += n
	return func() { m.used -= n }, nil
}

func TestRenderMemoryBudget(t *testing.T) {
	p := openPage(t, pageObjs("1 0 0 rg 0 0 100 100 re f"))
	mem := &testMemory{limit: 1}
	_, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Memory: mem})
	if !errors.Is(err, pdf.ErrBudget) {
		t.Fatalf("got %v", err)
	}
	mem.limit = 1 << 20
	_, err = RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Memory: mem})
	if err != nil {
		t.Fatal(err)
	}
	if mem.used != 0 {
		t.Fatalf("lease leaked: %d", mem.used)
	}
}

func TestRenderSeparateFillStrokeAlpha(t *testing.T) {
	objs := pageObjs("q /GS1 gs 0 0 1 RG 1 0 0 rg 0 0 100 100 re f 10 w 0 0 100 100 re S Q",
		testkit.Raw(5, "<< /Type /ExtGState /ca 0.5 /CA 1 >>"),
	)
	objs[2] = testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /ExtGState << /GS1 5 0 R >> >> /Contents 4 0 R >>")
	p := openPage(t, objs)
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := img.At(50, 50).RGBA()
	if r>>8 != 255 || g>>8 < 100 || g>>8 > 160 || b>>8 < 100 || b>>8 > 160 {
		t.Fatalf("fill alpha %v %v %v", r>>8, g>>8, b>>8)
	}
	_, _, b, _ = img.At(50, 2).RGBA()
	if b>>8 < 200 {
		t.Fatalf("stroke alpha %v", b>>8)
	}
}

func TestRenderImageLiveMemory(t *testing.T) {
	objs := pageObjs("q 10 0 0 10 0 0 cm /Im1 Do Q",
		testkit.Stream(5, "<< /Type /XObject /Subtype /Image /Width 10 /Height 10 /BitsPerComponent 8 /ColorSpace /DeviceRGB >>", make([]byte, 300)),
	)
	objs[2] = testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /XObject << /Im1 5 0 R >> >> /Contents 4 0 R >>")
	data := testkit.Build(objs, "/Root 1 0 R")
	b := pdf.DefaultBudget
	b.MaxLiveBytes = int64(len(data)) + 100
	d, err := pdf.Open(context.Background(), bytes.NewReader(data), int64(len(data)), pdf.WithBudget(&b))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	p, err := d.Page(0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72})
	if !errors.Is(err, pdf.ErrBudget) {
		t.Fatalf("got %v", err)
	}

	b.MaxLiveBytes = int64(len(data)) + 10_000
	d2, err := pdf.Open(context.Background(), bytes.NewReader(data), int64(len(data)), pdf.WithBudget(&b))
	if err != nil {
		t.Fatal(err)
	}
	defer d2.Close()
	p2, _ := d2.Page(0)
	if _, err := RenderRegion(context.Background(), p2, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72}); err != nil {
		t.Fatal(err)
	}
	if got := d2.ResourceUsage().LiveBytes; got != int64(len(data)) {
		t.Fatalf("lease leaked: live=%d want=%d", got, len(data))
	}
}

func TestRenderRasterLease(t *testing.T) {
	p := openPage(t, pageObjs("1 0 0 rg 0 0 100 100 re f"))
	mem := &testMemory{limit: 1 << 20}
	r, err := RenderPageLease(context.Background(), p, PageOptions{DPI: 72, Memory: mem})
	if err != nil {
		t.Fatal(err)
	}
	if mem.used < 100*100*4 {
		t.Fatalf("output not leased: %d", mem.used)
	}
	r.Release()
	r.Release()
	if mem.used != 0 {
		t.Fatalf("lease leaked: %d", mem.used)
	}
}

func TestRenderPagesExecutor(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Contents 5 0 R >>"),
		testkit.Raw(4, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Contents 6 0 R >>"),
		testkit.Stream(5, "", []byte("1 0 0 rg 0 0 100 100 re f")),
		testkit.Stream(6, "", []byte("0 1 0 rg 0 0 100 100 re f")),
	}
	data := testkit.Build(objs, "/Root 1 0 R")
	d, err := pdf.Open(context.Background(), bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	base := d.ResourceUsage().LiveBytes
	results, err := RenderPages(context.Background(), d, []int{1, 0}, PageOptions{DPI: 72}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Index != 1 || results[1].Index != 0 || results[0].Err != nil || results[1].Err != nil {
		t.Fatalf("results %#v", results)
	}
	if d.ResourceUsage().LiveBytes <= base {
		t.Fatalf("outputs not leased: %#v", d.ResourceUsage())
	}
	for _, r := range results {
		r.Raster.Release()
	}
	if got := d.ResourceUsage().LiveBytes; got != base {
		t.Fatalf("lease leaked: got %d want %d", got, base)
	}
}

func TestRenderTiles(t *testing.T) {
	p := openPage(t, pageObjs("1 0 0 rg 0 0 100 100 re f"))
	var count, maxX, maxY int
	err := RenderTiles(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72}, 32, func(tile Tile) error {
		count++
		if tile.X > maxX {
			maxX = tile.X
		}
		if tile.Y > maxY {
			maxY = tile.Y
		}
		if tile.X == 0 && tile.Y == 0 {
			r, g, b, _ := tile.Image.At(1, 1).RGBA()
			if r>>8 != 255 || g>>8 != 0 || b>>8 != 0 {
				t.Fatalf("tile pixel %v %v %v", r>>8, g>>8, b>>8)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 16 || maxX != 96 || maxY != 96 {
		t.Fatalf("tiles count=%d maxX=%d maxY=%d", count, maxX, maxY)
	}
}

func TestTilePlan(t *testing.T) {
	plan, err := PlanTiles(displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72}, 32)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Width != 100 || plan.Height != 100 || plan.Columns != 4 || plan.Rows != 4 || plan.Scale != 1 {
		t.Fatalf("plan %#v", plan)
	}
}

func TestAdaptiveTilesAndAssembly(t *testing.T) {
	plan, err := PlanTilesAdaptive(displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72}, 32*32*4)
	if err != nil {
		t.Fatal(err)
	}
	if plan.TilePixels != 32 || plan.Columns != 4 || plan.Rows != 4 {
		t.Fatalf("plan %#v", plan)
	}
	p := openPage(t, pageObjs("1 0 0 rg 0 0 100 100 re f"))
	dst := image.NewRGBA(image.Rect(0, 0, 100, 100))
	if err := RenderPageTilesTo(context.Background(), p, PageOptions{DPI: 72, Background: "white"}, 32, dst); err != nil {
		t.Fatal(err)
	}
	if r, g, b, _ := dst.At(50, 50).RGBA(); r>>8 != 255 || g>>8 != 0 || b>>8 != 0 {
		t.Fatalf("assembled pixel %v %v %v", r>>8, g>>8, b>>8)
	}
}

func TestRenderDeviceGrayGroupBlendSpace(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /XObject << /Fm1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/Fm1 Do")),
		testkit.Stream(5, "<< /Type /XObject /Subtype /Form /BBox [0 0 100 100] /Group << /S /Transparency /I true /CS /DeviceGray >> >>", []byte("1 0 0 rg 0 0 100 100 re f")),
	}
	data := testkit.Build(objs, "/Root 1 0 R")
	d, err := pdf.Open(context.Background(), bytes.NewReader(data), int64(len(data)), pdf.WithPreserveForms(true))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	p, _ := d.Page(0)
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := img.At(50, 50).RGBA()
	if r>>8 != g>>8 || g>>8 != b>>8 || r>>8 == 255 {
		t.Fatalf("not gray: %v %v %v", r>>8, g>>8, b>>8)
	}
}

func TestRenderNonIsolatedDeviceGrayGroup(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /XObject << /Fm1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/Fm1 Do")),
		testkit.Stream(5, "<< /Type /XObject /Subtype /Form /BBox [0 0 100 100] /Group << /S /Transparency /I false /CS /DeviceGray >> >>", []byte("1 0 0 rg 0 0 50 100 re f")),
	}
	data := testkit.Build(objs, "/Root 1 0 R")
	d, err := pdf.Open(context.Background(), bytes.NewReader(data), int64(len(data)), pdf.WithPreserveForms(true))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	p, _ := d.Page(0)
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := img.At(25, 50).RGBA()
	if r>>8 != g>>8 || g>>8 != b>>8 {
		t.Fatalf("not gray: %v %v %v", r>>8, g>>8, b>>8)
	}
}

func TestRenderKnockoutGroup(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /XObject << /Fm1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/Fm1 Do")),
		testkit.Stream(5, "<< /Type /XObject /Subtype /Form /BBox [0 0 100 100] /Group << /S /Transparency /I true /K true >> /Resources << /ExtGState << /GS1 6 0 R >> >> >>", []byte("1 0 0 rg 0 0 50 100 re f q /GS1 gs 0 0 1 rg 25 0 50 100 re f Q")),
		testkit.Raw(6, "<< /Type /ExtGState /ca 0.5 >>"),
	}
	data := testkit.Build(objs, "/Root 1 0 R")
	d, err := pdf.Open(context.Background(), bytes.NewReader(data), int64(len(data)), pdf.WithPreserveForms(true))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	p, _ := d.Page(0)
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	r, _, b, _ := img.At(50, 50).RGBA()
	if r>>8 > 160 || b>>8 < 200 {
		t.Fatalf("knockout pixel %v %v", r>>8, b>>8)
	}
}

func TestGrayImageAlpha(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	src.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 128})
	c := grayImage{src}.At(0, 0).(color.NRGBA)
	if c.A != 128 || c.R < 70 || c.R > 82 || c.R != c.G || c.G != c.B {
		t.Fatalf("gray %#v", c)
	}
}

func TestTextStrokeMode(t *testing.T) {
	tr := &displaylist.TextRun{
		Common:     displaylist.Common{Alpha: 1, Clip: -1, Bounds: displaylist.Rect{X0: 20, Y0: 40, X1: 30, Y1: 50}},
		Font:       &displaylist.FontInfo{GlyphSource: fakeGlyphs{}},
		Size:       10,
		RenderMode: 1,
		Stroke:     &displaylist.Paint{Kind: displaylist.PaintSolid, RGBA: [4]float64{1, 0, 0, 1}},
		Line:       displaylist.LineStyle{Width: 2},
		Glyphs: []displaylist.Glyph{{
			Code: 1, Matrix: displaylist.Matrix{10, 0, 0, -10, 20, 50},
		}},
	}
	rr := &regionRenderer{
		ctx: context.Background(), out: image.NewRGBA(image.Rect(0, 0, 100, 100)),
		box: displaylist.Rect{X0: 0, Y0: 0, X1: 100, Y1: 100}, scale: 1,
		pageToPx: displaylist.Matrix{1, 0, 0, 1, 0, 0}, clips: map[int]*image.Alpha{},
		softMasks: map[*displaylist.SoftMask]*image.Alpha{}, memory: noMemory{},
	}
	if err := rr.text(tr); err != nil {
		t.Fatal(err)
	}
	found := false
	for y := 0; y < 100 && !found; y++ {
		for x := 0; x < 100; x++ {
			if rr.out.RGBAAt(x, y).A != 0 {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("text stroke produced no pixels")
	}
}

func TestClipIntersection(t *testing.T) {
	rect := func(x0, y0, x1, y1 float64) []displaylist.Segment {
		return []displaylist.Segment{
			{Kind: displaylist.SegmentMove, P0: displaylist.Point{X: x0, Y: y0}},
			{Kind: displaylist.SegmentLine, P0: displaylist.Point{X: x0, Y: y0}, P3: displaylist.Point{X: x1, Y: y0}},
			{Kind: displaylist.SegmentLine, P0: displaylist.Point{X: x1, Y: y0}, P3: displaylist.Point{X: x1, Y: y1}},
			{Kind: displaylist.SegmentLine, P0: displaylist.Point{X: x1, Y: y1}, P3: displaylist.Point{X: x0, Y: y1}},
			{Kind: displaylist.SegmentClose},
		}
	}
	big := displaylist.Clip{Path: rect(0, 0, 10, 10), Bounds: displaylist.Rect{X0: 0, Y0: 0, X1: 10, Y1: 10}}
	small := displaylist.Clip{Path: rect(2, 2, 4, 4), Bounds: displaylist.Rect{X0: 2, Y0: 2, X1: 4, Y1: 4}, Intersect: []displaylist.Clip{big}}
	rr := &regionRenderer{
		ctx: context.Background(), out: image.NewRGBA(image.Rect(0, 0, 10, 10)),
		box: displaylist.Rect{X0: 0, Y0: 0, X1: 10, Y1: 10}, scale: 1,
		pageToPx: displaylist.Matrix{1, 0, 0, 1, 0, 0}, clips: map[int]*image.Alpha{},
		clipList: []displaylist.Clip{big, small}, softMasks: map[*displaylist.SoftMask]*image.Alpha{},
		memory: noMemory{},
	}
	m := rr.clipMask(1)
	if m == nil {
		t.Fatal("nil clip mask")
	}
	if m.AlphaAt(3, 3).A == 0 || m.AlphaAt(1, 1).A != 0 || m.AlphaAt(5, 5).A != 0 {
		t.Fatalf("clip alpha = %v %v %v", m.AlphaAt(3, 3).A, m.AlphaAt(1, 1).A, m.AlphaAt(5, 5).A)
	}
}

func TestKnockoutPreservesTransparentHoles(t *testing.T) {
	rect := func(x0, y0, x1, y1 float64) []displaylist.Segment {
		return []displaylist.Segment{
			{Kind: displaylist.SegmentMove, P0: displaylist.Point{X: x0, Y: y0}},
			{Kind: displaylist.SegmentLine, P0: displaylist.Point{X: x0, Y: y0}, P3: displaylist.Point{X: x1, Y: y0}},
			{Kind: displaylist.SegmentLine, P0: displaylist.Point{X: x1, Y: y0}, P3: displaylist.Point{X: x1, Y: y1}},
			{Kind: displaylist.SegmentLine, P0: displaylist.Point{X: x1, Y: y1}, P3: displaylist.Point{X: x0, Y: y1}},
			{Kind: displaylist.SegmentClose},
		}
	}
	first := &displaylist.Path{
		Common:   displaylist.Common{Alpha: 1, Clip: -1, Bounds: displaylist.Rect{X0: 0, Y0: 0, X1: 20, Y1: 20}},
		Segments: rect(0, 0, 20, 20), Fill: &displaylist.Paint{Kind: displaylist.PaintSolid, RGBA: [4]float64{1, 0, 0, 1}}, Closed: true,
	}
	second := &displaylist.Path{
		Common:   displaylist.Common{Alpha: 1, Clip: -1, Bounds: displaylist.Rect{X0: 0, Y0: 0, X1: 20, Y1: 20}},
		Segments: append(rect(0, 0, 20, 20), rect(5, 5, 15, 15)...), Fill: &displaylist.Paint{Kind: displaylist.PaintSolid, RGBA: [4]float64{0, 0, 1, 1}},
		FillRule: displaylist.EvenOdd, Closed: true,
	}
	g := &displaylist.Group{
		Common:   displaylist.Common{Alpha: 1, Clip: -1, Bounds: displaylist.Rect{X0: 0, Y0: 0, X1: 20, Y1: 20}},
		Isolated: true, Knockout: true, Items: []displaylist.Item{first, second},
	}
	rr := &regionRenderer{
		ctx: context.Background(), out: image.NewRGBA(image.Rect(0, 0, 20, 20)),
		box: displaylist.Rect{X0: 0, Y0: 0, X1: 20, Y1: 20}, scale: 1,
		pageToPx: displaylist.Matrix{1, 0, 0, 1, 0, 0}, clips: map[int]*image.Alpha{},
		softMasks: map[*displaylist.SoftMask]*image.Alpha{}, memory: noMemory{},
	}
	if err := rr.group(g); err != nil {
		t.Fatal(err)
	}
	if got := rr.out.RGBAAt(10, 10); got != (color.RGBA{R: 255, A: 255}) {
		t.Fatalf("hole = %#v, want red", got)
	}
	if got := rr.out.RGBAAt(2, 2); got != (color.RGBA{B: 255, A: 255}) {
		t.Fatalf("ring = %#v, want blue", got)
	}
}

func TestKnockoutNonNormalBlendUsesBackdrop(t *testing.T) {
	p := &displaylist.Path{
		Common: displaylist.Common{Alpha: 1, Clip: -1, Bounds: displaylist.Rect{X0: 0, Y0: 0, X1: 10, Y1: 10}, Blend: "Multiply"},
		Segments: []displaylist.Segment{
			{Kind: displaylist.SegmentMove, P0: displaylist.Point{X: 0, Y: 0}},
			{Kind: displaylist.SegmentLine, P0: displaylist.Point{X: 0, Y: 0}, P3: displaylist.Point{X: 10, Y: 0}},
			{Kind: displaylist.SegmentLine, P0: displaylist.Point{X: 10, Y: 0}, P3: displaylist.Point{X: 10, Y: 10}},
			{Kind: displaylist.SegmentLine, P0: displaylist.Point{X: 10, Y: 10}, P3: displaylist.Point{X: 0, Y: 10}},
			{Kind: displaylist.SegmentClose},
		},
		Fill: &displaylist.Paint{Kind: displaylist.PaintSolid, RGBA: [4]float64{0.5, 0.5, 0.5, 1}}, Closed: true,
	}
	g := &displaylist.Group{
		Common:   displaylist.Common{Alpha: 1, Clip: -1, Bounds: displaylist.Rect{X0: 0, Y0: 0, X1: 10, Y1: 10}},
		Knockout: true, Items: []displaylist.Item{p},
	}
	rr := &regionRenderer{
		ctx: context.Background(), out: image.NewRGBA(image.Rect(0, 0, 10, 10)),
		box: displaylist.Rect{X0: 0, Y0: 0, X1: 10, Y1: 10}, scale: 1,
		pageToPx: displaylist.Matrix{1, 0, 0, 1, 0, 0}, clips: map[int]*image.Alpha{},
		softMasks: map[*displaylist.SoftMask]*image.Alpha{}, memory: noMemory{},
	}
	draw.Draw(rr.out, rr.out.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	if err := rr.group(g); err != nil {
		t.Fatal(err)
	}
	got := rr.out.RGBAAt(5, 5)
	if got.R < 120 || got.R > 135 || got.G != got.R || got.B != got.R || got.A != 255 {
		t.Fatalf("multiply result = %#v", got)
	}
}

func TestRendererProfileAndCapabilities(t *testing.T) {
	p := openPage(t, pageObjs("0 0 50 50 re f"))
	r, err := NewRenderer(Options{Profile: RenderProfile{Name: "test", DPI: 72, IncludeText: true}})
	if err != nil {
		t.Fatal(err)
	}
	caps, err := r.Capabilities(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if caps.Status != RenderableFull || caps.EffectiveProfile != "test" {
		t.Fatalf("caps = %#v", caps)
	}
	rp, err := r.RenderPage(context.Background(), p, PageOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer rp.Close()
	if b := rp.Image.Bounds(); b.Dx() != 100 || b.Dy() != 100 {
		t.Fatalf("bounds = %v", b)
	}
	var buf bytes.Buffer
	if err := r.EncodeTo(context.Background(), &buf, p, PageOptions{}, "png", 0); err != nil {
		t.Fatal(err)
	}
	if buf.Len() == 0 {
		t.Fatal("empty PNG")
	}
}

func TestRendererCapabilitiesMissingOutlines(t *testing.T) {
	objs := pageObjs("BT /F1 12 Tf 10 50 Td (Hi) Tj ET",
		testkit.Raw(5, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"),
	)
	objs[2] = testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>")
	p := openPage(t, objs)
	r := &Renderer{opts: Options{Profile: ProfilePreview}}
	caps, err := r.Capabilities(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if caps.Status != RenderableApproximate || caps.MissingTextOutlines == 0 {
		t.Fatalf("caps = %#v", caps)
	}
	found := false
	for _, issue := range caps.Issues {
		if issue.Code == "RND-CAP-TEXT-NO-OUTLINE" {
			found = true
		}
	}
	if !found {
		t.Fatalf("issues = %#v", caps.Issues)
	}
}

type testFontSource []byte

func (s testFontSource) ReadAt(p []byte, off int64) (int, error) {
	return bytes.NewReader(s).ReadAt(p, off)
}
func (s testFontSource) Size() int64 { return int64(len(s)) }

type testFontEngine struct {
	loads int
	opens int
}

func (e *testFontEngine) Info() extension.ProviderInfo {
	return extension.ProviderInfo{Name: "test-font", Version: "1", License: "MIT", Determinism: extension.Deterministic}
}
func (*testFontEngine) Features() []extension.Feature { return nil }
func (e *testFontEngine) Load(_ context.Context, src io.ReaderAt, size int64, spec extension.FontSpec) (extension.FontFace, error) {
	e.loads++
	if src == nil || size == 0 || spec.BaseFont != "Test" {
		return nil, errors.New("bad font load")
	}
	return &testFontFace{engine: e}, nil
}

type testFontFace struct{ engine *testFontEngine }

func (*testFontFace) Metrics() extension.FontMetrics { return extension.FontMetrics{} }
func (*testFontFace) Glyph(context.Context, uint32, uint32) (extension.Glyph, bool, error) {
	return extension.Glyph{Outline: []extension.OutlineSegment{{
		Op: extension.OutlineCubic,
		P0: extension.Point{X: 1, Y: 2},
		P1: extension.Point{X: 3, Y: 4},
		P2: extension.Point{X: 5, Y: 6},
		P3: extension.Point{X: 7, Y: 8},
	}}}, true, nil
}
func (f *testFontFace) Close() error { f.engine.opens++; return nil }

func TestExtensionFontProvider(t *testing.T) {
	engine := &testFontEngine{}
	p := newExtensionFontProvider(context.Background(), engine)
	info := &displaylist.FontInfo{BaseFont: "Test", Kind: "TrueType", Embedded: true, Source: testFontSource("font")}
	seg, ok, err := p.GlyphOutline(info, displaylist.Glyph{Code: 1, CID: 2})
	if err != nil || !ok || len(seg) != 1 {
		t.Fatalf("outline = %v %v %v", seg, ok, err)
	}
	if seg[0].Kind != displaylist.SegmentCubic || seg[0].P3.X != 7 || seg[0].P3.Y != 8 {
		t.Fatalf("outline = %#v", seg[0])
	}
	if _, _, err := p.GlyphOutline(info, displaylist.Glyph{Code: 1}); err != nil {
		t.Fatal(err)
	}
	if engine.loads != 1 {
		t.Fatalf("loads = %d", engine.loads)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if engine.opens != 1 {
		t.Fatalf("opens = %d", engine.opens)
	}
}

func TestExtensionFontProviderRequiresSource(t *testing.T) {
	p := newExtensionFontProvider(context.Background(), &testFontEngine{})
	if _, ok, err := p.GlyphOutline(&displaylist.FontInfo{BaseFont: "Test"}, displaylist.Glyph{}); ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestRenderHighlyCompressibleImage(t *testing.T) {
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	row := make([]byte, 256*256*3)
	for i := 0; i < len(row); i += 3 {
		row[i] = 255
	}
	if _, err := zw.Write(row); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /XObject << /Im1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("q 100 0 0 100 0 0 cm /Im1 Do Q")),
		testkit.Stream(5, fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width 256 /Height 256 /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /FlateDecode /Length %d >>", z.Len()), z.Bytes()),
	}
	data := testkit.Build(objs, "/Root 1 0 R")
	d, err := pdf.Open(context.Background(), bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	p, _ := d.Page(0)
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, _ := img.At(50, 50).RGBA(); r>>8 != 255 || g>>8 != 0 || b>>8 != 0 {
		t.Fatalf("solid image pixel %v", img.At(50, 50))
	}
}

func TestRenderIndexedImage(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Resources << /XObject << /Im1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("q 10 0 0 10 0 0 cm /Im1 Do Q")),
		testkit.Stream(5, "<< /Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace 6 0 R /BitsPerComponent 8 /Length 4 >>", []byte{0, 1, 1, 0}),
		testkit.Raw(6, "[/Indexed /DeviceRGB 1 <FF000000FF00>]"),
	}
	p := openPage(t, objs)
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 10, Y1: 10}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, _ := img.At(2, 2).RGBA(); r>>8 != 255 || g>>8 != 0 || b>>8 != 0 {
		t.Fatalf("indexed red = %v", img.At(2, 2))
	}
	if r, g, b, _ := img.At(7, 2).RGBA(); r>>8 != 0 || g>>8 != 255 || b>>8 != 0 {
		t.Fatalf("indexed green = %v", img.At(7, 2))
	}
	if r, g, b, _ := img.At(2, 7).RGBA(); r>>8 != 0 || g>>8 != 255 || b>>8 != 0 {
		t.Fatalf("indexed green = %v", img.At(2, 7))
	}
	if r, g, b, _ := img.At(7, 7).RGBA(); r>>8 != 255 || g>>8 != 0 || b>>8 != 0 {
		t.Fatalf("indexed red = %v", img.At(7, 7))
	}
}

func TestRenderPageUsesRotatedPageSpace(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /CropBox [10 20 190 80] /Rotate 90 /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("10 20 180 60 re f")),
	}
	p := openPage(t, objs)
	img, err := RenderPage(context.Background(), p, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 60 || b.Dy() != 180 {
		t.Fatalf("rotated page bounds = %v", b)
	}
	if r, g, b, _ := img.At(30, 90).RGBA(); r>>8 != 0 || g>>8 != 0 || b>>8 != 0 {
		t.Fatalf("crop/rotation fill = %v", img.At(30, 90))
	}
}

func TestBuiltinFontFallback(t *testing.T) {
	objs := pageObjs("BT /F1 50 Tf 10 20 Td (Hi) Tj ET")
	objs[2] = testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>")
	objs = append(objs, testkit.Raw(5, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"))
	p := openPage(t, objs)
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{
		DPI: 72, Background: "white", IncludeText: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	ink := 0
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r>>8 < 250 || g>>8 < 250 || b>>8 < 250 {
				ink++
			}
		}
	}
	if ink == 0 {
		t.Fatal("builtin fallback produced no text pixels")
	}
	if _, ok, err := BuiltinFonts().GlyphOutline(&displaylist.FontInfo{BaseFont: "Helvetica"}, displaylist.Glyph{Text: "H"}); err != nil || !ok {
		t.Fatalf("builtin H outline = %v, %v", ok, err)
	}
}

type emptyFontProvider struct{}

func (emptyFontProvider) GlyphOutline(*displaylist.FontInfo, displaylist.Glyph) ([]displaylist.Segment, bool, error) {
	return nil, false, nil
}

type errorFontProvider struct{}

func (errorFontProvider) GlyphOutline(*displaylist.FontInfo, displaylist.Glyph) ([]displaylist.Segment, bool, error) {
	return nil, false, errors.New("font failure")
}

func TestBuiltinFontVariantsAndChain(t *testing.T) {
	for _, info := range []displaylist.FontInfo{
		{BaseFont: "Helvetica"},
		{BaseFont: "Helvetica-Bold", Bold: true},
		{BaseFont: "Helvetica-Oblique", Italic: true},
		{BaseFont: "Helvetica-BoldOblique", Bold: true, Italic: true},
		{BaseFont: "Courier", Mono: true},
		{BaseFont: "Courier-Bold", Mono: true, Bold: true},
		{BaseFont: "Courier-Oblique", Mono: true, Italic: true},
		{BaseFont: "Courier-BoldOblique", Mono: true, Bold: true, Italic: true},
	} {
		if seg, ok, err := BuiltinFonts().GlyphOutline(&info, displaylist.Glyph{Text: "H"}); err != nil || !ok || len(seg) == 0 {
			t.Fatalf("variant %s: %v, %v, %d", info.BaseFont, ok, err, len(seg))
		}
	}
	chain := ChainFonts(nil, emptyFontProvider{}, rectangleFontProvider{})
	if seg, ok, err := chain.GlyphOutline(&displaylist.FontInfo{}, displaylist.Glyph{Text: "H"}); err != nil || !ok || len(seg) == 0 {
		t.Fatalf("chain fallback = %v, %v, %d", ok, err, len(seg))
	}
	chain = ChainFonts(emptyFontProvider{}, errorFontProvider{}, rectangleFontProvider{})
	if _, ok, err := chain.GlyphOutline(&displaylist.FontInfo{}, displaylist.Glyph{Text: "H"}); err == nil || ok {
		t.Fatalf("chain error = %v, %v", ok, err)
	}
}

func TestRenderPagesEach(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R 4 0 R 5 0 R] /Count 3 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Contents 6 0 R >>"),
		testkit.Raw(4, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Contents 7 0 R >>"),
		testkit.Raw(5, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Contents 8 0 R >>"),
		testkit.Stream(6, "", []byte("1 0 0 rg 0 0 100 100 re f")),
		testkit.Stream(7, "", []byte("0 1 0 rg 0 0 100 100 re f")),
		testkit.Stream(8, "", []byte("0 0 1 rg 0 0 100 100 re f")),
	}
	data := testkit.Build(objs, "/Root 1 0 R")
	d, err := pdf.Open(context.Background(), bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	base := d.ResourceUsage().LiveBytes
	seen := map[int]bool{}
	var mu sync.Mutex
	err = RenderPagesEach(context.Background(), d, nil, PageOptions{DPI: 72}, 3, func(res PageResult) error {
		if res.Err != nil || res.Raster == nil || res.Raster.Image.Bounds().Dx() != 100 {
			t.Errorf("result %#v", res)
		}
		mu.Lock()
		seen[res.Index] = true
		mu.Unlock()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 3 || !seen[0] || !seen[1] || !seen[2] {
		t.Fatalf("seen %v", seen)
	}
	if got := d.ResourceUsage().LiveBytes; got != base {
		t.Fatalf("lease leaked: got %d want %d", got, base)
	}
}

func TestRenderPagesEachCallbackError(t *testing.T) {
	data := testkit.Build(pageObjs("1 0 0 rg 0 0 100 100 re f"), "/Root 1 0 R")
	d, err := pdf.Open(context.Background(), bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	boom := errors.New("stop")
	calls := 0
	err = RenderPagesEach(context.Background(), d, []int{0}, PageOptions{DPI: 72}, 1, func(PageResult) error {
		calls++
		return boom
	})
	if !errors.Is(err, boom) || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestRenderPageTilesRotated(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /CropBox [10 20 190 80] /Rotate 90 /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("10 20 180 60 re f")),
	}
	p := openPage(t, objs)
	dst := image.NewRGBA(image.Rect(0, 0, 60, 180))
	if err := RenderPageTilesTo(context.Background(), p, PageOptions{DPI: 72, Background: "white"}, 32, dst); err != nil {
		t.Fatal(err)
	}
	if r, g, b, _ := dst.At(30, 90).RGBA(); r>>8 != 0 || g>>8 != 0 || b>>8 != 0 {
		t.Fatalf("rotated tile fill = %v", dst.At(30, 90))
	}
	count := 0
	if err := RenderPageTiles(context.Background(), p, PageOptions{DPI: 72}, 32, func(tile Tile) error {
		count++
		if tile.X < 0 || tile.Y < 0 || tile.X >= 60 || tile.Y >= 180 {
			t.Fatalf("tile origin %d,%d", tile.X, tile.Y)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if count != 12 {
		t.Fatalf("tiles = %d", count)
	}
}

func TestRotatedTextAdvanceAndTd(t *testing.T) {
	objs := pageObjs("BT /F1 36 Tf 0 1 -1 0 50 50 Tm (A) Tj 0 -10 Td (B) Tj ET")
	objs[2] = testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>")
	objs = append(objs, testkit.Raw(5, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"))
	p := openPage(t, objs)
	var run *displaylist.TextRun
	_ = p.Walk(context.Background(), func(it displaylist.Item) error {
		if tr, ok := it.(*displaylist.TextRun); ok {
			run = tr
		}
		return nil
	})
	if run == nil || len(run.Glyphs) != 2 {
		t.Fatalf("run = %#v", run)
	}
	dx := run.Glyphs[1].Origin.X - run.Glyphs[0].Origin.X
	dy := run.Glyphs[1].Origin.Y - run.Glyphs[0].Origin.Y
	if math.Abs(dx-10) > 0.01 || math.Abs(dy) > 0.01 {
		t.Fatalf("rotated Td delta = %.3f,%.3f", dx, dy)
	}
}

func TestRotate90CornerMapping(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Rotate 90 /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("1 0 0 rg 0 0 20 20 re f 0 1 0 rg 180 0 20 20 re f 0 0 1 rg 0 80 20 20 re f 1 1 0 rg 180 80 20 20 re f")),
	}
	p := openPage(t, objs)
	img, err := RenderPage(context.Background(), p, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		x, y int
		want color.RGBA
	}{
		{10, 10, color.RGBA{R: 255, A: 255}},
		{90, 10, color.RGBA{B: 255, A: 255}},
		{10, 190, color.RGBA{G: 255, A: 255}},
		{90, 190, color.RGBA{R: 255, G: 255, A: 255}},
	}
	for _, tc := range cases {
		if got := img.At(tc.x, tc.y).(color.RGBA); got != tc.want {
			t.Fatalf("pixel %d,%d = %v, want %v", tc.x, tc.y, got, tc.want)
		}
	}
}

func TestRenderShadingPattern(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /Pattern << /P1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/Pattern cs /P1 scn 0 0 100 100 re f")),
		testkit.Raw(5, "<< /Type /Pattern /PatternType 2 /Shading 6 0 R >>"),
		testkit.Raw(6, "<< /ShadingType 2 /ColorSpace /DeviceRGB /Coords [0 0 100 0] /Function 7 0 R /Extend [true true] >>"),
		testkit.Raw(7, "<< /FunctionType 2 /Domain [0 1] /C0 [0 0 0] /C1 [1 1 1] /N 1 >>"),
	}
	p := openPage(t, objs)
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	l, _, _, _ := img.At(10, 50).RGBA()
	r, _, _, _ := img.At(90, 95).RGBA()
	if l>>8 > 40 || r>>8 < 200 {
		t.Fatalf("shading pattern endpoints = %d, %d", l>>8, r>>8)
	}
}

func TestRenderRadialShadingPattern(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /Pattern << /P1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/Pattern cs /P1 scn 0 0 100 100 re f")),
		testkit.Raw(5, "<< /Type /Pattern /PatternType 2 /Shading 6 0 R >>"),
		testkit.Raw(6, "<< /ShadingType 3 /ColorSpace /DeviceRGB /Coords [50 50 0 50 50 40] /Function 7 0 R /Extend [true true] >>"),
		testkit.Raw(7, "<< /FunctionType 2 /Domain [0 1] /C0 [0 0 0] /C1 [1 1 1] /N 1 >>"),
	}
	p := openPage(t, objs)
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	center, _, _, _ := img.At(50, 50).RGBA()
	edge, _, _, _ := img.At(95, 50).RGBA()
	if center>>8 > 40 || edge>>8 < 180 {
		t.Fatalf("radial pattern endpoints = %d, %d", center>>8, edge>>8)
	}
}

func TestRenderShadingPatternSoftMask(t *testing.T) {
	objs := pageObjs("/GS1 gs /Pattern cs /P1 scn 0 0 100 100 re f",
		testkit.Raw(5, "<< /Type /ExtGState /SMask 8 0 R >>"),
		testkit.Raw(6, "<< /Type /Pattern /PatternType 2 /Shading 7 0 R >>"),
		testkit.Raw(7, "<< /ShadingType 2 /ColorSpace /DeviceRGB /Coords [0 0 100 0] /Function 9 0 R /Extend [true true] >>"),
		testkit.Stream(8, "<< /Type /XObject /Subtype /Form /BBox [0 0 100 100] /Group << /S /Transparency /CS /DeviceGray >> >>", []byte("1 1 1 rg 0 0 50 100 re f")),
		testkit.Raw(9, "<< /FunctionType 2 /Domain [0 1] /C0 [1 0 0] /C1 [1 0 0] /N 1 >>"),
	)
	objs[2] = testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /ExtGState << /GS1 5 0 R >> /Pattern << /P1 6 0 R >> >> /Contents 4 0 R >>")
	p := openPage(t, objs)
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := img.At(10, 50).RGBA()
	if r>>8 < 200 || g>>8 > 160 || b>>8 > 160 {
		t.Fatalf("soft-masked pattern inside = %d,%d,%d", r>>8, g>>8, b>>8)
	}
	if r, g, b, _ := img.At(90, 95).RGBA(); r>>8 != 255 || g>>8 != 255 || b>>8 != 255 {
		t.Fatalf("soft-masked pattern outside = %d,%d,%d", r>>8, g>>8, b>>8)
	}
}

func TestRenderTilingPatternSoftMask(t *testing.T) {
	objs := pageObjs("/GS1 gs /Pattern cs /P1 scn 0 0 100 100 re f",
		testkit.Raw(5, "<< /Type /ExtGState /SMask 8 0 R >>"),
		testkit.Stream(6, "<< /Type /Pattern /PatternType 1 /PaintType 1 /TilingType 1 /BBox [0 0 10 10] /XStep 10 /YStep 10 /Resources << >> >>", []byte("1 0 0 rg 0 0 10 10 re f")),
		testkit.Stream(8, "<< /Type /XObject /Subtype /Form /BBox [0 0 100 100] /Group << /S /Transparency /CS /DeviceGray >> >>", []byte("1 1 1 rg 0 0 50 100 re f")),
	)
	objs[2] = testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /ExtGState << /GS1 5 0 R >> /Pattern << /P1 6 0 R >> >> /Contents 4 0 R >>")
	p := openPage(t, objs)
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := img.At(25, 50).RGBA()
	if r>>8 < 200 || g>>8 > 160 || b>>8 > 160 {
		t.Fatalf("soft-masked tiling pattern inside = %d,%d,%d", r>>8, g>>8, b>>8)
	}
	if r, g, b, _ := img.At(90, 95).RGBA(); r>>8 != 255 || g>>8 != 255 || b>>8 != 255 {
		t.Fatalf("soft-masked tiling pattern outside = %d,%d,%d", r>>8, g>>8, b>>8)
	}
}

func TestRenderTilingPatternRepeats(t *testing.T) {
	objs := pageObjs("/Pattern cs /P1 scn 0 0 100 100 re f",
		testkit.Stream(6, "<< /Type /Pattern /PatternType 1 /PaintType 1 /TilingType 1 /BBox [0 0 10 10] /XStep 10 /YStep 10 /Resources << >> >>", []byte("1 0 0 rg 0 0 5 5 re f")),
	)
	objs[2] = testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /Pattern << /P1 6 0 R >> >> /Contents 4 0 R >>")
	p := openPage(t, objs)
	img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white"})
	if err != nil {
		t.Fatal(err)
	}
	for _, pt := range []image.Point{{X: 2, Y: 98}, {X: 22, Y: 98}, {X: 2, Y: 78}, {X: 22, Y: 78}} {
		if r, g, b, _ := img.At(pt.X, pt.Y).RGBA(); r>>8 != 255 || g>>8 != 0 || b>>8 != 0 {
			t.Fatalf("tile paint at %v = %d,%d,%d", pt, r>>8, g>>8, b>>8)
		}
	}
}

func TestRenderTransparencyGroupAlpha(t *testing.T) {
	objs := pageObjs("/GS1 gs /Fm1 Do",
		testkit.Raw(5, "<< /Type /ExtGState /ca 0.5 /CA 0.5 >>"),
		testkit.Stream(6, "<< /Type /XObject /Subtype /Form /BBox [0 0 100 100] /Group 7 0 R >>", []byte("1 0 0 rg 0 0 100 100 re f")),
		testkit.Raw(7, "<< /S /Transparency /I false >>"),
	)
	objs[2] = testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /ExtGState << /GS1 5 0 R >> /XObject << /Fm1 6 0 R >> >> /Contents 4 0 R >>")
	data := testkit.Build(objs, "/Root 1 0 R")
	for _, preserve := range []bool{false, true} {
		d, err := pdf.Open(context.Background(), bytes.NewReader(data), int64(len(data)), pdf.WithPreserveForms(preserve))
		if err != nil {
			t.Fatal(err)
		}
		p, _ := d.Page(0)
		img, err := RenderRegion(context.Background(), p, displaylist.Rect{X1: 100, Y1: 100}, PageOptions{DPI: 72, Background: "white"})
		_ = d.Close()
		if err != nil {
			t.Fatal(err)
		}
		r, g, b, _ := img.At(50, 50).RGBA()
		if r>>8 < 240 || g>>8 < 100 || g>>8 > 160 || b>>8 < 100 || b>>8 > 160 {
			t.Fatalf("group alpha pixel (preserve=%v) = %d,%d,%d", preserve, r>>8, g>>8, b>>8)
		}
	}
}

func TestBlendPremultipliedAlpha(t *testing.T) {
	out := image.NewRGBA(image.Rect(0, 0, 1, 1))
	out.SetRGBA(0, 0, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	rr := &regionRenderer{out: out}
	rr.blendRGBA(0, 0, color.RGBA{R: 255, G: 255, B: 255, A: 235}, "Normal")
	got := out.RGBAAt(0, 0)
	if got.A < 254 || got.R > got.A || got.G > got.A || got.B > got.A {
		t.Fatalf("premultiplied pixel = %v", got)
	}
	drawPixel(out, 0, 0, color.RGBA{R: 255, G: 255, B: 255, A: 128})
	if got = out.RGBAAt(0, 0); got.A == 0 {
		t.Fatalf("drawPixel produced empty pixel")
	}
}
