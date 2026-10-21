package render

import (
	"bytes"
	"context"
	"errors"
	"image/color"
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
