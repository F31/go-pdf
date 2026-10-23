package render

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"
	"testing"

	"github.com/F31/go-pdf"
	"github.com/F31/go-pdf/displaylist"
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
		{1, AnnotationPrint, false},
		{4, AnnotationPrint, true},
		{0, AnnotationPrint, false},
		{32, AnnotationView, false},
		{36, AnnotationPrint, true},
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

func TestRenderNonIsolatedDeviceCMYKGroup(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /XObject << /Fm1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/Fm1 Do")),
		testkit.Stream(5, "<< /Type /XObject /Subtype /Form /BBox [0 0 100 100] /Group << /S /Transparency /I false /CS /DeviceCMYK >> >>", []byte("1 0 0 rg 0 0 100 100 re f")),
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
	if r>>8 < 240 || g>>8 > 15 || b>>8 > 15 {
		t.Fatalf("cmyk group pixel %v %v %v", r>>8, g>>8, b>>8)
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

type sampleImageRef struct{ s *displaylist.ImageSamples }

func (s sampleImageRef) Raw() ([]byte, error) { return nil, nil }
func (s sampleImageRef) Decode(context.Context) (image.Image, error) {
	return image.NewRGBA(image.Rect(0, 0, 1, 1)), nil
}
func (s sampleImageRef) DecodeSamples(context.Context) (*displaylist.ImageSamples, error) {
	return s.s, nil
}

func TestGroupImageSamplesUseSourceCMYK(t *testing.T) {
	ref := sampleImageRef{s: displaylist.NewImageSamples(1, 1, 4, "DeviceCMYK", nil, []float32{0, 1, 0, 0}, nil)}
	im := &displaylist.Image{
		Common: displaylist.Common{Alpha: 1, Clip: -1},
		Matrix: displaylist.Matrix{1, 0, 0, 1, 0, 0},
		Info:   displaylist.ImageInfo{Width: 1, Height: 1, ColorSpace: "DeviceCMYK"},
		Data:   ref,
	}
	rr := &regionRenderer{
		ctx: context.Background(), out: image.NewRGBA(image.Rect(0, 0, 2, 2)),
		box: displaylist.Rect{X0: 0, Y0: 0, X1: 2, Y1: 2}, scale: 1,
		pageToPx: displaylist.Matrix{1, 0, 0, 1, 0, 0}, clips: map[int]*image.Alpha{},
		softMasks: map[*displaylist.SoftMask]*image.Alpha{}, memory: noMemory{},
	}
	acc, alpha, release, ok, err := rr.groupImageSamples(im, cmykConverter{}, image.Rect(0, 0, 2, 2), 4, 2, 2)
	if err != nil || !ok || release == nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	defer release()
	if alpha[0] != 1 {
		t.Fatalf("alpha = %v", alpha[0])
	}
	got := []float32{acc[0], acc[1], acc[2], acc[3]}
	want := []float32{0, 1, 0, 0}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("acc = %v", got)
		}
	}
}

func TestGroupDeviceUsesImageSourceSamples(t *testing.T) {
	ref := sampleImageRef{s: displaylist.NewImageSamples(1, 1, 4, "DeviceCMYK", nil, []float32{0, 1, 1, 0}, nil)}
	im := &displaylist.Image{
		Common: displaylist.Common{Alpha: 1, Clip: -1},
		Matrix: displaylist.Matrix{1, 0, 0, 1, 0, 0},
		Info:   displaylist.ImageInfo{Width: 1, Height: 1, ColorSpace: "DeviceCMYK"},
		Data:   ref,
	}
	g := &displaylist.Group{Common: displaylist.Common{Clip: -1, Bounds: displaylist.Rect{X0: 0, Y0: 0, X1: 2, Y1: 2}}, ColorSpace: "DeviceCMYK", BlendColorSpace: "DeviceCMYK", Items: []displaylist.Item{im}}
	rr := &regionRenderer{
		ctx: context.Background(), out: image.NewRGBA(image.Rect(0, 0, 2, 2)),
		box: displaylist.Rect{X0: 0, Y0: 0, X1: 2, Y1: 2}, scale: 1,
		pageToPx: displaylist.Matrix{1, 0, 0, 1, 0, 0}, clips: map[int]*image.Alpha{},
		softMasks: map[*displaylist.SoftMask]*image.Alpha{}, memory: noMemory{},
	}
	done, err := rr.groupDevice(g, cmykConverter{})
	if err != nil || !done {
		t.Fatalf("done=%v err=%v", done, err)
	}
	if got := rr.out.RGBAAt(0, 0); got != (color.RGBA{R: 255, A: 255}) {
		t.Fatalf("pixel = %#v", got)
	}
}

type countConverter struct{ calls int }

func (c *countConverter) components() int { return 1 }
func (c *countConverter) fromSRGB(rgba [4]float64) ([]float64, error) {
	c.calls++
	return []float64{(rgba[0] + rgba[1] + rgba[2]) / 3}, nil
}
func (c *countConverter) toSRGB(vals []float64) ([4]float64, error) {
	return [4]float64{vals[0], vals[0], vals[0], 1}, nil
}

func TestGroupDeviceSplitsPathFillAndStroke(t *testing.T) {
	seg := []displaylist.Segment{
		{Kind: displaylist.SegmentMove, P0: displaylist.Point{X: 0, Y: 0}},
		{Kind: displaylist.SegmentLine, P0: displaylist.Point{X: 1, Y: 0}, P3: displaylist.Point{X: 1, Y: 1}},
		{Kind: displaylist.SegmentClose},
	}
	p := &displaylist.Path{
		Common:   displaylist.Common{Alpha: 1, Clip: -1, Bounds: displaylist.Rect{X0: 0, Y0: 0, X1: 1, Y1: 1}},
		Segments: seg,
		Fill:     &displaylist.Paint{Kind: displaylist.PaintSolid, RGBA: [4]float64{0, 1, 0, 1}},
		Stroke:   &displaylist.Paint{Kind: displaylist.PaintSolid, RGBA: [4]float64{0, 0, 1, 1}},
		Line:     displaylist.LineStyle{Width: 1},
	}
	g := &displaylist.Group{Common: displaylist.Common{Clip: -1, Bounds: displaylist.Rect{X0: 0, Y0: 0, X1: 2, Y1: 2}}, Isolated: true, Items: []displaylist.Item{p}}
	rr := &regionRenderer{
		ctx: context.Background(), out: image.NewRGBA(image.Rect(0, 0, 2, 2)),
		box: displaylist.Rect{X0: 0, Y0: 0, X1: 2, Y1: 2}, scale: 1,
		pageToPx: displaylist.Matrix{1, 0, 0, 1, 0, 0}, clips: map[int]*image.Alpha{},
		softMasks: map[*displaylist.SoftMask]*image.Alpha{}, memory: noMemory{},
		opts: PageOptions{IncludeText: true},
	}
	conv := &countConverter{}
	done, err := rr.groupDevice(g, conv)
	if err != nil || !done {
		t.Fatalf("done=%v err=%v", done, err)
	}
	if conv.calls < 2 {
		t.Fatalf("fromSRGB calls = %d", conv.calls)
	}
}

type fakeGlyphs struct{}

func (fakeGlyphs) GlyphOutline(code, cid uint32) ([]displaylist.Segment, bool) {
	return []displaylist.Segment{
		{Kind: displaylist.SegmentMove, P0: displaylist.Point{X: 0, Y: 0}},
		{Kind: displaylist.SegmentLine, P0: displaylist.Point{X: 1000, Y: 0}, P3: displaylist.Point{X: 1000, Y: 1000}},
		{Kind: displaylist.SegmentLine, P0: displaylist.Point{X: 1000, Y: 1000}, P3: displaylist.Point{X: 0, Y: 1000}},
		{Kind: displaylist.SegmentClose},
	}, true
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

func TestGroupDeviceRendersTextPerGlyph(t *testing.T) {
	tr := &displaylist.TextRun{
		Common:     displaylist.Common{Alpha: 1, Clip: -1, Bounds: displaylist.Rect{X0: 10, Y0: 40, X1: 50, Y1: 60}},
		Font:       &displaylist.FontInfo{GlyphSource: fakeGlyphs{}},
		Size:       10,
		RenderMode: 0,
		Fill:       displaylist.Paint{Kind: displaylist.PaintSolid, RGBA: [4]float64{1, 1, 1, 1}},
		Glyphs: []displaylist.Glyph{
			{Code: 1, Matrix: displaylist.Matrix{10, 0, 0, -10, 20, 50}},
			{Code: 2, Matrix: displaylist.Matrix{10, 0, 0, -10, 40, 50}},
		},
	}
	g := &displaylist.Group{Common: displaylist.Common{Clip: -1, Bounds: displaylist.Rect{X0: 0, Y0: 0, X1: 80, Y1: 80}}, Isolated: true, Items: []displaylist.Item{tr}}
	rr := &regionRenderer{
		ctx: context.Background(), out: image.NewRGBA(image.Rect(0, 0, 80, 80)),
		box: displaylist.Rect{X0: 0, Y0: 0, X1: 80, Y1: 80}, scale: 1,
		pageToPx: displaylist.Matrix{1, 0, 0, 1, 0, 0}, clips: map[int]*image.Alpha{},
		softMasks: map[*displaylist.SoftMask]*image.Alpha{}, memory: noMemory{},
		opts: PageOptions{IncludeText: true},
	}
	conv := &countConverter{}
	done, err := rr.groupDevice(g, conv)
	if err != nil || !done {
		t.Fatalf("done=%v err=%v", done, err)
	}
	if conv.calls == 0 {
		t.Fatal("no device-space glyph conversion")
	}
	count := 0
	minX, minY, maxX, maxY := 1000, 1000, -1, -1
	for y := 0; y < 80; y++ {
		for x := 0; x < 80; x++ {
			if rr.out.RGBAAt(x, y).A != 0 {
				count++
				if x < minX {
					minX = x
				}
				if y < minY {
					minY = y
				}
				if x > maxX {
					maxX = x
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if count == 0 {
		t.Fatal("no glyph pixels rendered")
	}
	t.Logf("glyph pixels=%d bounds=(%d,%d)-(%d,%d)", count, minX, minY, maxX, maxY)
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
		Common:   displaylist.Common{Clip: -1, Bounds: displaylist.Rect{X0: 0, Y0: 0, X1: 20, Y1: 20}},
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
		Common:   displaylist.Common{Clip: -1, Bounds: displaylist.Rect{X0: 0, Y0: 0, X1: 10, Y1: 10}},
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
	r, err := NewRenderer(Options{Profile: ProfilePreview})
	if err != nil {
		t.Fatal(err)
	}
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
