package pdf

import (
	"bytes"
	"context"
	"fmt"
	stdimage "image"
	stdcolor "image/color"
	"image/jpeg"
	"io"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/extension"
	"github.com/F31/go-pdf/internal/testkit"
	pdftext "github.com/F31/go-pdf/text"
)

func contentDoc(t *testing.T, content string) *Document {
	t.Helper()
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte(content)),
		testkit.Raw(5, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"),
	}
	return openTest(t, testkit.Build(objs, "/Root 1 0 R"))
}

func TestTextRunPositioning(t *testing.T) {
	d := contentDoc(t, "BT /F1 12 Tf 10 20 Td (Hello) Tj ET")
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dl.Items) != 1 {
		t.Fatalf("items %d", len(dl.Items))
	}
	tr, ok := dl.Items[0].(*displaylist.TextRun)
	if !ok {
		t.Fatalf("type %T", dl.Items[0])
	}
	if len(tr.Glyphs) != 5 {
		t.Fatalf("glyphs %d", len(tr.Glyphs))
	}
	g := tr.Glyphs[0]
	if math.Abs(g.Origin.X-10) > 0.001 || math.Abs(g.Origin.Y-80) > 0.001 {
		t.Fatalf("origin %v,%v", g.Origin.X, g.Origin.Y)
	}
	if !g.UnicodeOK || g.Text != "H" {
		t.Fatalf("glyph0 %q ok=%v", g.Text, g.UnicodeOK)
	}
	if tr.Common.Z != 0 || tr.Common.ID != 0 {
		t.Fatalf("z/id %d/%d", tr.Common.Z, tr.Common.ID)
	}
	var text string
	for _, gl := range tr.Glyphs {
		text += gl.Text
	}
	if text != "Hello" {
		t.Fatalf("text %q", text)
	}
}

func TestTJAdjustment(t *testing.T) {
	d := contentDoc(t, "BT /F1 10 Tf 10 50 Td [(A) 100 (B)] TJ ET")
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	tr := dl.Items[0].(*displaylist.TextRun)
	if len(tr.Glyphs) != 2 {
		t.Fatalf("glyphs %d", len(tr.Glyphs))
	}
	dx := tr.Glyphs[1].Origin.X - tr.Glyphs[0].Origin.X
	want := 667.0/1000*10 - 1
	if math.Abs(dx-want) > 0.01 {
		t.Fatalf("dx %v want %v", dx, want)
	}
}

func TestPathAndClip(t *testing.T) {
	d := contentDoc(t, "1 0 0 rg 10 10 50 20 re f")
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dl.Items) != 1 {
		t.Fatalf("items %d", len(dl.Items))
	}
	path := dl.Items[0].(*displaylist.Path)
	if path.Fill == nil || path.Fill.RGBA[0] != 1 || path.Fill.RGBA[1] != 0 {
		t.Fatalf("fill %#v", path.Fill)
	}
	if math.Abs(path.Bounds.X0-10) > 0.001 || math.Abs(path.Bounds.Y0-70) > 0.001 || math.Abs(path.Bounds.X1-60) > 0.001 || math.Abs(path.Bounds.Y1-90) > 0.001 {
		t.Fatalf("bounds %#v", path.Bounds)
	}
	if path.Common.Clip != -1 {
		t.Fatalf("clip %d", path.Common.Clip)
	}
}

func TestClipIndex(t *testing.T) {
	d := contentDoc(t, "0 0 100 100 re W n 10 10 20 20 re f")
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dl.Clips) != 1 {
		t.Fatalf("clips %d", len(dl.Clips))
	}
	if len(dl.Items) != 1 {
		t.Fatalf("items %d", len(dl.Items))
	}
	if dl.Items[0].Base().Clip != 0 {
		t.Fatalf("clip index %d", dl.Items[0].Base().Clip)
	}
}

func TestWalkDisplayListConsistent(t *testing.T) {
	d := contentDoc(t, "BT /F1 10 Tf 10 50 Td (Hi) Tj ET 0 0 10 10 re f")
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var walked []displaylist.Item
	if err := p.Walk(context.Background(), func(it displaylist.Item) error {
		walked = append(walked, it)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(walked) != len(dl.Items) {
		t.Fatalf("walk %d display %d", len(walked), len(dl.Items))
	}
	for i := range walked {
		if walked[i].Base().Z != dl.Items[i].Base().Z || walked[i].Base().ID != dl.Items[i].Base().ID {
			t.Fatalf("item %d differs", i)
		}
	}
}

func TestDisplayListTextGolden(t *testing.T) {
	d := contentDoc(t, "BT /F1 12 Tf 10 20 Td (Hello) Tj ET")
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b, err := displaylist.CanonicalJSON(dl)
	if err != nil {
		t.Fatal(err)
	}
	testkit.GoldenJSON(t, "testdata/goldens/displaylist-text.json", b)
}

func TestTextMetricsGolden(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 200] /Resources << /Font << /F1 5 0 R /F2 6 0 R /F3 7 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("BT /F1 12 Tf 10 150 Td (Hello) Tj ET BT /F2 12 Tf 10 100 Td (Hello) Tj ET BT /F3 12 Tf 10 50 Td (Hello) Tj ET")),
		testkit.Raw(5, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"),
		testkit.Raw(6, "<< /Type /Font /Subtype /Type1 /BaseFont /Times-Roman >>"),
		testkit.Raw(7, "<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>"),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, it := range dl.Items {
		tr, ok := it.(*displaylist.TextRun)
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "font=%s size=%.0f\n", tr.Font.BaseFont, tr.Size)
		for _, g := range tr.Glyphs {
			fmt.Fprintf(&b, "  %s x=%.4f y=%.4f adv=%.4f\n", g.Text, g.Origin.X, g.Origin.Y, g.Advance)
		}
	}
	testkit.Golden(t, "testdata/goldens/text-metrics-base14.txt", []byte(b.String()))
}

func TestPathGolden(t *testing.T) {
	d := contentDoc(t, "0 0 100 100 re 10 10 m 20 20 l S")
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, it := range dl.Items {
		path, ok := it.(*displaylist.Path)
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "path z=%d clip=%d fill=%v stroke=%v bounds=%.2f,%.2f,%.2f,%.2f\n",
			path.Z, path.Clip, path.Fill != nil, path.Stroke != nil,
			path.Bounds.X0, path.Bounds.Y0, path.Bounds.X1, path.Bounds.Y1)
		for _, s := range path.Segments {
			fmt.Fprintf(&b, "  kind=%d p0=%.2f,%.2f p1=%.2f,%.2f p2=%.2f,%.2f p3=%.2f,%.2f\n",
				s.Kind, s.P0.X, s.P0.Y, s.P1.X, s.P1.Y, s.P2.X, s.P2.Y, s.P3.X, s.P3.Y)
		}
	}
	testkit.Golden(t, "testdata/goldens/path-rect-line.txt", []byte(b.String()))
}

func TestType0ToUnicodeIntegration(t *testing.T) {
	cmap := `
begincmap
1 begincodespacerange
<0000> <FFFF>
endcodespacerange
3 beginbfchar
<0001> <4E2D>
<0002> <6587>
<0010> <4E00>
endbfchar
endcmap
`
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("BT /F1 12 Tf 10 80 Td <00010002> Tj ET")),
		testkit.Raw(5, "<< /Type /Font /Subtype /Type0 /BaseFont /SimSun /Encoding /Identity-H /DescendantFonts [6 0 R] /ToUnicode 7 0 R >>"),
		testkit.Raw(6, "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /SimSun /DW 1000 /W [1 [500 600]] >>"),
		testkit.Stream(7, "", []byte(cmap)),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dl.Items) != 1 {
		t.Fatalf("items %d", len(dl.Items))
	}
	tr := dl.Items[0].(*displaylist.TextRun)
	if len(tr.Glyphs) != 2 {
		t.Fatalf("glyphs %d", len(tr.Glyphs))
	}
	if tr.Glyphs[0].Text != "中" || !tr.Glyphs[0].UnicodeOK || tr.Glyphs[0].Code != 1 || tr.Glyphs[0].CID != 1 {
		t.Fatalf("glyph0 %#v", tr.Glyphs[0])
	}
	if tr.Glyphs[1].Text != "文" || !tr.Glyphs[1].UnicodeOK || tr.Glyphs[1].Code != 2 {
		t.Fatalf("glyph1 %#v", tr.Glyphs[1])
	}
	if math.Abs(tr.Glyphs[0].Advance-6) > 0.001 || math.Abs(tr.Glyphs[1].Advance-7.2) > 0.001 {
		t.Fatalf("advances %v %v", tr.Glyphs[0].Advance, tr.Glyphs[1].Advance)
	}
	if math.Abs(tr.Glyphs[0].Origin.X-10) > 0.001 || math.Abs(tr.Glyphs[0].Origin.Y-20) > 0.001 {
		t.Fatalf("origin %v,%v", tr.Glyphs[0].Origin.X, tr.Glyphs[0].Origin.Y)
	}
}

func TestType0PredefinedCMapIntegration(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("BT /F1 10 Tf 10 80 Td <D6D0CEC4> Tj ET")),
		testkit.Raw(5, "<< /Type /Font /Subtype /Type0 /BaseFont /SimSun /Encoding /GBK-EUC-H /DescendantFonts [6 0 R] >>"),
		testkit.Raw(6, "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /SimSun /DW 1000 /W [4559 [900] 3795 [800]] >>"),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	tr := dl.Items[0].(*displaylist.TextRun)
	if len(tr.Glyphs) != 2 {
		t.Fatalf("glyphs %d", len(tr.Glyphs))
	}
	if tr.Glyphs[0].Text != "中" || !tr.Glyphs[0].UnicodeOK || tr.Glyphs[0].CID != 4559 {
		t.Fatalf("glyph0 %#v", tr.Glyphs[0])
	}
	if tr.Glyphs[1].Text != "文" || !tr.Glyphs[1].UnicodeOK || tr.Glyphs[1].CID != 3795 {
		t.Fatalf("glyph1 %#v", tr.Glyphs[1])
	}
	if math.Abs(tr.Glyphs[0].Advance-9) > 0.001 || math.Abs(tr.Glyphs[1].Advance-8) > 0.001 {
		t.Fatalf("advances %v %v", tr.Glyphs[0].Advance, tr.Glyphs[1].Advance)
	}
}

func TestIndirectFontResources(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font 6 0 R >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("BT /F1 12 Tf 10 80 Td (Hi) Tj ET")),
		testkit.Raw(5, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"),
		testkit.Raw(6, "<< /F1 5 0 R >>"),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	tr := dl.Items[0].(*displaylist.TextRun)
	if len(tr.Glyphs) != 2 || tr.Glyphs[0].Text != "H" || !tr.Glyphs[0].UnicodeOK {
		t.Fatalf("glyphs %#v", tr.Glyphs)
	}
}

func TestType3GlyphExpansion(t *testing.T) {
	cmap := `
begincmap
1 begincodespacerange
<00> <FF>
endcodespacerange
1 beginbfchar
<41> <4E2D>
endbfchar
endcmap
`
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("BT /F1 10 Tf 10 20 Td (A) Tj ET")),
		testkit.Raw(5, "<< /Type /Font /Subtype /Type3 /FontBBox [0 0 1000 1000] /FontMatrix [0.001 0 0 0.001 0 0] /CharProcs << /gA 7 0 R >> /Encoding << /Type /Encoding /Differences [65 /gA] >> /FirstChar 65 /LastChar 65 /Widths [1000] /ToUnicode 8 0 R >>"),
		testkit.Stream(7, "", []byte("1000 0 0 0 1000 1000 d1 0 0 1000 1000 re f")),
		testkit.Stream(8, "", []byte(cmap)),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dl.Items) != 2 {
		t.Fatalf("items %d", len(dl.Items))
	}
	path, ok := dl.Items[0].(*displaylist.Path)
	if !ok {
		t.Fatalf("item0 %T", dl.Items[0])
	}
	if path.Flags&displaylist.FlagFromType3 == 0 {
		t.Fatalf("missing FromType3 flag: %#v", path.Flags)
	}
	if math.Abs(path.Bounds.X0-10) > 0.01 || math.Abs(path.Bounds.Y0-70) > 0.01 ||
		math.Abs(path.Bounds.X1-20) > 0.01 || math.Abs(path.Bounds.Y1-80) > 0.01 {
		t.Fatalf("path bounds %#v", path.Bounds)
	}
	tr, ok := dl.Items[1].(*displaylist.TextRun)
	if !ok {
		t.Fatalf("item1 %T", dl.Items[1])
	}
	if tr.Font.Kind != "Type3" || len(tr.Glyphs) != 1 {
		t.Fatalf("font=%#v glyphs=%d", tr.Font, len(tr.Glyphs))
	}
	if tr.Glyphs[0].Text != "中" || !tr.Glyphs[0].UnicodeOK || math.Abs(tr.Glyphs[0].Advance-10) > 0.001 {
		t.Fatalf("glyph %#v", tr.Glyphs[0])
	}
}

func TestType3TextClipUsesCharProc(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("q BT /F1 10 Tf 7 Tr 10 20 Td (A) Tj ET 0 0 100 100 re f Q")),
		testkit.Raw(5, "<< /Type /Font /Subtype /Type3 /FontBBox [0 0 1000 1000] /FontMatrix [0.001 0 0 0.001 0 0] /CharProcs << /gA 7 0 R >> /Encoding << /Type /Encoding /Differences [65 /gA] >> /FirstChar 65 /LastChar 65 /Widths [1000] >>"),
		testkit.Stream(7, "", []byte("1000 0 0 0 1000 1000 d1 0 0 500 1000 re f")),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dl.Clips) != 1 {
		t.Fatalf("clips = %d", len(dl.Clips))
	}
	b := dl.Clips[0].Bounds
	if math.Abs(b.X0-10) > 0.01 || math.Abs(b.X1-15) > 0.01 {
		t.Fatalf("Type3 clip bounds %#v", b)
	}
	for _, dg := range d.Diagnostics() {
		if dg.Code == "CNT-TEXT-CLIP-BBOX" {
			t.Fatal("unexpected em-box fallback")
		}
	}
}

func TestEmbeddedCMapIntegration(t *testing.T) {
	encoding := `
begincmap
1 begincodespacerange
<0000> <FFFF>
endcodespacerange
1 begincidrange
<0001> <0002> 500
endcidrange
endcmap
`
	cmap := `
begincmap
1 begincodespacerange
<0000> <FFFF>
endcodespacerange
2 beginbfchar
<0001> <4E2D>
<0002> <6587>
endbfchar
endcmap
`
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("BT /F1 10 Tf 10 80 Td <00010002> Tj ET")),
		testkit.Raw(5, "<< /Type /Font /Subtype /Type0 /BaseFont /SimSun /Encoding 7 0 R /DescendantFonts [6 0 R] /ToUnicode 8 0 R >>"),
		testkit.Raw(6, "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /SimSun /DW 1000 /W [500 [700 800]] >>"),
		testkit.Stream(7, "", []byte(encoding)),
		testkit.Stream(8, "", []byte(cmap)),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	tr := dl.Items[0].(*displaylist.TextRun)
	if len(tr.Glyphs) != 2 {
		t.Fatalf("glyphs %d", len(tr.Glyphs))
	}
	if tr.Glyphs[0].Text != "中" || tr.Glyphs[0].CID != 500 || math.Abs(tr.Glyphs[0].Advance-7) > 0.001 {
		t.Fatalf("glyph0 %#v", tr.Glyphs[0])
	}
	if tr.Glyphs[1].Text != "文" || tr.Glyphs[1].CID != 501 || math.Abs(tr.Glyphs[1].Advance-8) > 0.001 {
		t.Fatalf("glyph1 %#v", tr.Glyphs[1])
	}
}

func TestType0Big5Integration(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("BT /F1 10 Tf 10 80 Td <A4A4> Tj ET")),
		testkit.Raw(5, "<< /Type /Font /Subtype /Type0 /BaseFont /MingLiU /Encoding /B5-H /DescendantFonts [6 0 R] >>"),
		testkit.Raw(6, "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /MingLiU /DW 1000 /W [661 [700]] >>"),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	tr := dl.Items[0].(*displaylist.TextRun)
	if len(tr.Glyphs) != 1 {
		t.Fatalf("glyphs %d", len(tr.Glyphs))
	}
	if tr.Glyphs[0].Text != "中" || !tr.Glyphs[0].UnicodeOK || tr.Glyphs[0].CID != 661 {
		t.Fatalf("glyph %#v", tr.Glyphs[0])
	}
	if math.Abs(tr.Glyphs[0].Advance-7) > 0.001 {
		t.Fatalf("advance %v", tr.Glyphs[0].Advance)
	}
}

func TestType0ShiftJISAndKorean(t *testing.T) {
	tests := []struct {
		name string
		enc  string
		hex  string
		cid  uint32
		text string
	}{
		{"shift-jis", "90ms-RKSJ-H", "82A0", 843, "あ"},
		{"korean", "KSCms-UHC-H", "B0A1", 1086, "가"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := []testkit.Obj{
				testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
				testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
				testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>"),
				testkit.Stream(4, "", []byte("BT /F1 10 Tf 10 80 Td <"+tt.hex+"> Tj ET")),
				testkit.Raw(5, "<< /Type /Font /Subtype /Type0 /BaseFont /Test /Encoding /"+tt.enc+" /DescendantFonts [6 0 R] >>"),
				testkit.Raw(6, fmt.Sprintf("<< /Type /Font /Subtype /CIDFontType2 /BaseFont /Test /DW 1000 /W [%d [700]] >>", tt.cid)),
			}
			d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
			defer d.Close()
			p, _ := d.Page(0)
			dl, err := p.DisplayList(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			tr := dl.Items[0].(*displaylist.TextRun)
			if len(tr.Glyphs) != 1 {
				t.Fatalf("glyphs %d", len(tr.Glyphs))
			}
			if tr.Glyphs[0].Text != tt.text || !tr.Glyphs[0].UnicodeOK || tr.Glyphs[0].CID != tt.cid {
				t.Fatalf("glyph %#v", tr.Glyphs[0])
			}
			if math.Abs(tr.Glyphs[0].Advance-7) > 0.001 {
				t.Fatalf("advance %v", tr.Glyphs[0].Advance)
			}
		})
	}
}

func TestTextClipFallbackBox(t *testing.T) {
	d := contentDoc(t, "BT /F1 20 Tf 7 Tr 10 20 Td (H) Tj ET 0 0 100 100 re f")
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dl.Clips) != 1 || len(dl.Clips[0].Path) == 0 {
		t.Fatalf("clips = %#v", dl.Clips)
	}
	found := false
	for _, dg := range d.Diagnostics() {
		if dg.Code == "CNT-TEXT-CLIP-BBOX" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing fallback clip diagnostic")
	}
}

func TestTextClipRestoredByQ(t *testing.T) {
	fontData, err := os.ReadFile("testdata/fonts/gopdf-test-sans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("q BT /F1 20 Tf 7 Tr 10 20 Td (H) Tj ET Q 0 0 100 100 re f")),
		testkit.Raw(5, "<< /Type /Font /Subtype /TrueType /BaseFont /GoPDFTestSans /Encoding /WinAnsiEncoding /FontDescriptor 6 0 R >>"),
		testkit.Raw(6, "<< /Type /FontDescriptor /FontName /GoPDFTestSans /Flags 32 /FontBBox [0 0 1000 1000] /ItalicAngle 0 /Ascent 905 /Descent -212 /CapHeight 700 /StemV 80 /FontFile2 7 0 R >>"),
		testkit.Stream(7, "", fontData),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range dl.Items {
		if path, ok := it.(*displaylist.Path); ok && path.Clip != -1 {
			t.Fatalf("path clip after Q = %d", path.Clip)
		}
	}
}

func TestTextClipRenderingMode(t *testing.T) {
	fontData, err := os.ReadFile("testdata/fonts/gopdf-test-sans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("q BT /F1 20 Tf 7 Tr 10 20 Td (H) Tj ET 0 0 100 100 re f Q")),
		testkit.Raw(5, "<< /Type /Font /Subtype /TrueType /BaseFont /GoPDFTestSans /Encoding /WinAnsiEncoding /FontDescriptor 6 0 R >>"),
		testkit.Raw(6, "<< /Type /FontDescriptor /FontName /GoPDFTestSans /Flags 32 /FontBBox [0 0 1000 1000] /ItalicAngle 0 /Ascent 905 /Descent -212 /CapHeight 700 /StemV 80 /FontFile2 7 0 R >>"),
		testkit.Stream(7, "", fontData),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dl.Clips) != 1 {
		t.Fatalf("clips = %d", len(dl.Clips))
	}
	if len(dl.Clips[0].Path) == 0 {
		t.Fatal("empty text clip")
	}
	var path *displaylist.Path
	for _, it := range dl.Items {
		if v, ok := it.(*displaylist.Path); ok {
			path = v
		}
	}
	if path == nil || path.Clip != 0 {
		t.Fatalf("path clip = %#v", path)
	}
}

func TestFormXObject(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F1 6 0 R >> /XObject << /Fm1 7 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("q /Fm1 Do Q")),
		testkit.Raw(6, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"),
		testkit.Stream(7, "<< /Type /XObject /Subtype /Form /BBox [0 0 50 50] /Matrix [1 0 0 1 20 30] /Resources << /Font << /F1 6 0 R >> >> >>", []byte("BT /F1 10 Tf 0 0 Td (Hi) Tj ET")),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dl.Items) != 1 {
		t.Fatalf("items %d", len(dl.Items))
	}
	tr := dl.Items[0].(*displaylist.TextRun)
	if len(tr.Glyphs) != 2 || tr.Glyphs[0].Text != "H" || tr.Glyphs[1].Text != "i" {
		t.Fatalf("glyphs %#v", tr.Glyphs)
	}
	if math.Abs(tr.Glyphs[0].Origin.X-20) > 0.001 || math.Abs(tr.Glyphs[0].Origin.Y-70) > 0.001 {
		t.Fatalf("origin %#v", tr.Glyphs[0].Origin)
	}
	if tr.Clip != 0 || len(dl.Clips) != 1 {
		t.Fatalf("clip=%d clips=%d", tr.Clip, len(dl.Clips))
	}
	if b := dl.Clips[0].Bounds; math.Abs(b.X0-20) > 0.001 || math.Abs(b.Y0-20) > 0.001 || math.Abs(b.X1-70) > 0.001 || math.Abs(b.Y1-70) > 0.001 {
		t.Fatalf("clip bounds %#v", b)
	}
}

func TestFormXObjectClipIntersection(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /XObject << /Fm1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("q 0 0 50 50 re W n /Fm1 Do Q")),
		testkit.Stream(5, "<< /Type /XObject /Subtype /Form /BBox [0 0 100 100] /Resources << >> >>", []byte("q 0 0 20 20 re W n 0 0 10 10 re f Q")),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, err := d.Page(0)
	if err != nil {
		t.Fatal(err)
	}
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range dl.Items {
		path, ok := item.(*displaylist.Path)
		if !ok || path.Clip < 0 || path.Clip >= len(dl.Clips) {
			continue
		}
		found = true
		if len(dl.Clips[path.Clip].Intersect) == 0 {
			t.Fatalf("form clip was not intersected: %#v", dl.Clips[path.Clip])
		}
	}
	if !found {
		t.Fatal("form path not found")
	}
	for _, item := range d.Diagnostics() {
		if item.Code == "CNT-FORM-CLIP-INTERSECT" {
			t.Fatalf("unexpected approximate clip diagnostic: %#v", item)
		}
	}
}

func TestFormXObjectInheritsResources(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F1 6 0 R >> /XObject << /Fm1 7 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/Fm1 Do")),
		testkit.Raw(6, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"),
		testkit.Stream(7, "<< /Type /XObject /Subtype /Form /BBox [0 0 100 100] >>", []byte("BT /F1 10 Tf 0 0 Td (Ok) Tj ET")),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	tr := dl.Items[0].(*displaylist.TextRun)
	if len(tr.Glyphs) != 2 || tr.Glyphs[0].Text != "O" || tr.Glyphs[1].Text != "k" {
		t.Fatalf("glyphs %#v", tr.Glyphs)
	}
}

func TestFormXObjectCycle(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /XObject << /A 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/A Do")),
		testkit.Stream(5, "<< /Type /XObject /Subtype /Form /BBox [0 0 100 100] /Resources << /XObject << /B 6 0 R >> >> >>", []byte("/B Do")),
		testkit.Stream(6, "<< /Type /XObject /Subtype /Form /BBox [0 0 100 100] /Resources << /XObject << /A 5 0 R >> >> >>", []byte("/A Do")),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dl.Items) != 0 {
		t.Fatalf("items %d", len(dl.Items))
	}
	found := false
	for _, dg := range dl.Diag {
		if dg.Code == "CNT-FORM-CYCLE" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing cycle diagnostic: %#v", dl.Diag)
	}
}

func TestGraphicsMatrixOrder(t *testing.T) {
	d := contentDoc(t, "q 10 0 0 10 50 50 cm 0 0 1 1 re f Q")
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dl.Items) != 1 {
		t.Fatalf("items %d", len(dl.Items))
	}
	b := dl.Items[0].(*displaylist.Path).Bounds
	if math.Abs(b.X0-50) > 0.001 || math.Abs(b.Y0-40) > 0.001 || math.Abs(b.X1-60) > 0.001 || math.Abs(b.Y1-50) > 0.001 {
		t.Fatalf("bounds %#v", b)
	}
}

func TestImageXObject(t *testing.T) {
	raw := []byte{255, 0, 0, 0, 255, 0}
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /XObject << /Im1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("q 50 0 0 50 10 10 cm /Im1 Do Q")),
		testkit.Stream(5, "<< /Type /XObject /Subtype /Image /Width 2 /Height 1 /BitsPerComponent 8 /ColorSpace /DeviceRGB /Mask [0 0 0 0 0 0] >>", raw),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dl.Items) != 1 {
		t.Fatalf("items %d", len(dl.Items))
	}
	im := dl.Items[0].(*displaylist.Image)
	if im.Info.Width != 2 || im.Info.Height != 1 || im.Info.ColorSpace != "DeviceRGB" || im.Info.Encoding != "Raw" {
		t.Fatalf("info %#v", im.Info)
	}
	if im.Matrix != [6]float64{50, 0, 0, -50, 10, 90} {
		t.Fatalf("matrix %#v", im.Matrix)
	}
	if im.Bounds.X0 != 10 || im.Bounds.Y0 != 40 || im.Bounds.X1 != 60 || im.Bounds.Y1 != 90 {
		t.Fatalf("bounds %#v", im.Bounds)
	}
	if im.Mask == nil || !im.Mask.IsColorKey || len(im.Mask.ColorKey) != 6 {
		t.Fatalf("mask %#v", im.Mask)
	}
	gotRaw, err := im.Data.Raw()
	if err != nil || !bytes.Equal(gotRaw, raw) {
		t.Fatalf("raw %v %v", gotRaw, err)
	}
	img, err := im.Data.Decode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	rgba := img.(*stdimage.RGBA)
	if r, g, b, _ := rgba.At(0, 0).RGBA(); r>>8 != 255 || g>>8 != 0 || b>>8 != 0 {
		t.Fatalf("pixel0 %v", rgba.At(0, 0))
	}
	if r, g, b, _ := rgba.At(1, 0).RGBA(); r>>8 != 0 || g>>8 != 255 || b>>8 != 0 {
		t.Fatalf("pixel1 %v", rgba.At(1, 0))
	}
	imgs, err := p.Images(context.Background())
	if err != nil || len(imgs) != 1 || imgs[0].Width != 2 {
		t.Fatalf("images %#v %v", imgs, err)
	}
}

func TestInlineImage(t *testing.T) {
	d := contentDoc(t, "q 10 0 0 10 20 20 cm BI /W 1 /H 1 /BPC 8 /CS /G ID \x80 EI Q")
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dl.Items) != 1 {
		t.Fatalf("items %d", len(dl.Items))
	}
	im := dl.Items[0].(*displaylist.Image)
	if !im.Info.Inline || im.Info.Width != 1 || im.Info.Height != 1 || im.Info.ColorSpace != "DeviceGray" {
		t.Fatalf("info %#v", im.Info)
	}
	raw, err := im.Data.Raw()
	if err != nil || !bytes.Equal(raw, []byte{0x80}) {
		t.Fatalf("raw %v %v", raw, err)
	}
	img, err := im.Data.Decode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	gray := img.(*stdimage.Gray)
	if gray.GrayAt(0, 0).Y != 128 {
		t.Fatalf("gray %d", gray.GrayAt(0, 0).Y)
	}
}

func TestInlineASCIIHexImage(t *testing.T) {
	d := contentDoc(t, "BI /W 1 /H 1 /BPC 8 /CS /G /F /AHx ID 80> EI")
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	im := dl.Items[0].(*displaylist.Image)
	if im.Info.Encoding != "AHx" {
		t.Fatalf("encoding %q", im.Info.Encoding)
	}
	img, err := im.Data.Decode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if img.(*stdimage.Gray).GrayAt(0, 0).Y != 128 {
		t.Fatalf("gray %d", img.(*stdimage.Gray).GrayAt(0, 0).Y)
	}
}

func TestDCTImage(t *testing.T) {
	src := stdimage.NewRGBA(stdimage.Rect(0, 0, 1, 1))
	src.SetRGBA(0, 0, stdcolor.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /XObject << /Im1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/Im1 Do")),
		testkit.Stream(5, "<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /BitsPerComponent 8 /ColorSpace /DeviceRGB /Filter /DCTDecode >>", raw),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	im := dl.Items[0].(*displaylist.Image)
	if im.Info.Encoding != "DCTDecode" {
		t.Fatalf("encoding %q", im.Info.Encoding)
	}
	gotRaw, _ := im.Data.Raw()
	if !bytes.Equal(gotRaw, raw) {
		t.Fatal("raw JPEG differs")
	}
	img, err := im.Data.Decode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := img.At(0, 0).RGBA()
	if r>>8 < 200 || g>>8 > 80 || b>>8 > 80 {
		t.Fatalf("jpeg pixel0 rgba=(%d,%d,%d)", r>>8, g>>8, b>>8)
	}
}

func TestPageFonts(t *testing.T) {
	d := contentDoc(t, "BT /F1 12 Tf 10 20 Td (Hi) Tj ET")
	defer d.Close()
	p, _ := d.Page(0)
	fonts, err := p.Fonts(context.Background())
	if err != nil || len(fonts) != 1 || fonts[0].BaseFont != "Helvetica" {
		t.Fatalf("fonts %#v %v", fonts, err)
	}
}

func TestImageCMYKDecode(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /XObject << /Im1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/Im1 Do")),
		testkit.Stream(5, "<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /BitsPerComponent 8 /ColorSpace /DeviceCMYK /Decode [1 0 1 0 1 0 1 0] >>", []byte{0, 0, 0, 0}),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	im := dl.Items[0].(*displaylist.Image)
	img, err := im.Data.Decode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := img.At(0, 0).RGBA()
	if r>>8 != 0 || g>>8 != 0 || b>>8 != 0 {
		t.Fatalf("cmyk decode rgba=(%d,%d,%d)", r>>8, g>>8, b>>8)
	}
}

func TestImageMask(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /XObject << /Im1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/Im1 Do")),
		testkit.Stream(5, "<< /Type /XObject /Subtype /Image /Width 2 /Height 1 /ImageMask true >>", []byte{0x40}),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	im := dl.Items[0].(*displaylist.Image)
	if im.Info.ColorSpace != "ImageMask" || im.Info.BitsPerComponent != 1 || im.Role != displaylist.ImageMaskRole {
		t.Fatalf("info %#v role %d", im.Info, im.Role)
	}
	img, err := im.Data.Decode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	alpha := img.(*stdimage.Alpha)
	if alpha.AlphaAt(0, 0).A != 255 || alpha.AlphaAt(1, 0).A != 0 {
		t.Fatalf("mask alpha %d %d", alpha.AlphaAt(0, 0).A, alpha.AlphaAt(1, 0).A)
	}
}

func TestImageSMask(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /XObject << /Im1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/Im1 Do")),
		testkit.Stream(5, "<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /BitsPerComponent 8 /ColorSpace /DeviceRGB /SMask 6 0 R >>", []byte{255, 0, 0}),
		testkit.Stream(6, "<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /BitsPerComponent 8 /ColorSpace /DeviceGray >>", []byte{128}),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	im := dl.Items[0].(*displaylist.Image)
	if im.Mask == nil || im.Mask.Image == nil || im.Mask.IsColorKey {
		t.Fatalf("mask %#v", im.Mask)
	}
	img, err := im.Mask.Image.Decode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if img.(*stdimage.Gray).GrayAt(0, 0).Y != 128 {
		t.Fatalf("smask %d", img.(*stdimage.Gray).GrayAt(0, 0).Y)
	}
}

func TestTextPositionWithCM(t *testing.T) {
	d := contentDoc(t, "q 2 0 0 2 20 20 cm BT /F1 10 Tf 0 0 Td (H) Tj ET Q")
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	tr := dl.Items[0].(*displaylist.TextRun)
	if math.Abs(tr.Glyphs[0].Origin.X-20) > 0.001 || math.Abs(tr.Glyphs[0].Origin.Y-80) > 0.001 {
		t.Fatalf("origin %#v", tr.Glyphs[0].Origin)
	}
	if tr.Glyphs[0].Matrix != [6]float64{20, 0, 0, -20, 20, 80} {
		t.Fatalf("matrix %#v", tr.Glyphs[0].Matrix)
	}
}

func TestCCITTImage(t *testing.T) {
	raw := []byte{0xFF, 0x10, 0x01}
	for _, tc := range []struct {
		name  string
		black bool
		gray  uint8
	}{
		{"white", false, 255},
		{"black-is-one", true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			objs := []testkit.Obj{
				testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
				testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
				testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /XObject << /Im1 5 0 R >> >> /Contents 4 0 R >>"),
				testkit.Stream(4, "", []byte("/Im1 Do")),
				testkit.Stream(5, fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width 8 /Height 8 /BitsPerComponent 1 /ColorSpace /DeviceGray /Filter /CCITTFaxDecode /DecodeParms << /K -1 /Columns 8 /Rows 8 /BlackIs1 %t >> >>", tc.black), raw),
			}
			d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
			defer d.Close()
			p, _ := d.Page(0)
			dl, err := p.DisplayList(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			im := dl.Items[0].(*displaylist.Image)
			if im.Info.Encoding != "CCITTFaxDecode" {
				t.Fatalf("encoding %q", im.Info.Encoding)
			}
			img, err := im.Data.Decode(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			gray := img.(*stdimage.Gray)
			if gray.GrayAt(0, 0).Y != tc.gray || gray.GrayAt(7, 7).Y != tc.gray {
				t.Fatalf("gray %d %d", gray.GrayAt(0, 0).Y, gray.GrayAt(7, 7).Y)
			}
		})
	}
}

type fakeJBIG2Decoder struct {
	raw []byte
}

func (d *fakeJBIG2Decoder) Decode(ctx context.Context, raw []byte, info displaylist.ImageInfo) (stdimage.Image, error) {
	d.raw = append([]byte(nil), raw...)
	img := stdimage.NewGray(stdimage.Rect(0, 0, info.Width, info.Height))
	for y := 0; y < info.Height; y++ {
		for x := 0; x < info.Width; x++ {
			img.SetGray(x, y, stdcolor.Gray{Y: 77})
		}
	}
	return img, nil
}

type fakeJBIG2Factory struct{ dec *fakeJBIG2Decoder }

func (f fakeJBIG2Factory) Encodings() []displaylist.ImageEncoding {
	return []displaylist.ImageEncoding{"JBIG2Decode"}
}
func (f fakeJBIG2Factory) NewDecoder(ctx context.Context, info displaylist.ImageInfo) (ExternalDecoder, error) {
	return f.dec, nil
}

func TestExternalImageDecoder(t *testing.T) {
	dec := &fakeJBIG2Decoder{}
	reg, err := NewDecoderRegistry(fakeJBIG2Factory{dec: dec})
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte{9, 8, 7}
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /XObject << /Im1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/Im1 Do")),
		testkit.Stream(5, "<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /BitsPerComponent 1 /ColorSpace /DeviceGray /Filter /JBIG2Decode >>", raw),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"), WithDecoderRegistry(reg))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	im := dl.Items[0].(*displaylist.Image)
	img, err := im.Data.Decode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(dec.raw, raw) {
		t.Fatalf("external raw %v", dec.raw)
	}
	if img.(*stdimage.Gray).GrayAt(0, 0).Y != 77 {
		t.Fatalf("external pixel %d", img.(*stdimage.Gray).GrayAt(0, 0).Y)
	}
}

func TestCCITTG3Image(t *testing.T) {
	raw := []byte{
		0x00, 0x19, 0x80, 0x0c, 0xc0, 0x06, 0x60, 0x03, 0x30, 0x01,
		0x98, 0x00, 0xcc, 0x00, 0x66, 0x00, 0x33, 0x00, 0x10, 0x01,
		0x00, 0x10, 0x01, 0x00, 0x10, 0x01,
	}
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /XObject << /Im1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/Im1 Do")),
		testkit.Stream(5, "<< /Type /XObject /Subtype /Image /Width 8 /Height 8 /BitsPerComponent 1 /ColorSpace /DeviceGray /Filter /CCITTFaxDecode /DecodeParms << /K 0 /Columns 8 /Rows 8 /EndOfLine true >> >>", raw),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	im := dl.Items[0].(*displaylist.Image)
	img, err := im.Data.Decode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if img.(*stdimage.Gray).GrayAt(4, 4).Y != 255 {
		t.Fatalf("g3 gray %d", img.(*stdimage.Gray).GrayAt(4, 4).Y)
	}
}

func TestExtGState(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F1 5 0 R >> /ExtGState << /GS1 6 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("q /GS1 gs 0 0 10 10 re f 0 0 5 5 re S BT /F1 10 Tf 0 0 Td (Hi) Tj ET Q")),
		testkit.Raw(5, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"),
		testkit.Raw(6, "<< /Type /ExtGState /ca 0.5 /CA 0.25 /BM /Multiply /LW 2 /LC 1 /LJ 1 /ML 4 /D [[3 2] 1] >>"),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dl.Items) != 3 {
		t.Fatalf("items %d", len(dl.Items))
	}
	fill := dl.Items[0].(*displaylist.Path)
	if fill.Alpha != 0.5 || fill.Blend != "Multiply" || fill.Line.Width != 2 || fill.Line.Cap != 1 || fill.Line.Join != 1 || fill.Line.Miter != 4 {
		t.Fatalf("fill %#v line %#v", fill.Common, fill.Line)
	}
	if len(fill.Line.Dash) != 2 || fill.Line.Dash[0] != 3 || fill.Line.Phase != 1 {
		t.Fatalf("dash %#v phase %v", fill.Line.Dash, fill.Line.Phase)
	}
	stroke := dl.Items[1].(*displaylist.Path)
	if stroke.Alpha != 0.25 || stroke.Blend != "Multiply" {
		t.Fatalf("stroke %#v", stroke.Common)
	}
	text := dl.Items[2].(*displaylist.TextRun)
	if text.Alpha != 0.5 || text.Blend != "Multiply" {
		t.Fatalf("text %#v", text.Common)
	}
}

func TestExtGStateSoftMask(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /ExtGState << /GS1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/GS1 gs 1 0 0 rg 0 0 100 100 re f /GS1 gs 0 1 0 rg 0 0 100 100 re f")),
		testkit.Raw(5, "<< /Type /ExtGState /SMask 6 0 R >>"),
		testkit.Stream(6, "<< /Type /XObject /Subtype /Form /BBox [0 0 100 100] /Group << /S /Transparency /CS /DeviceGray >> /Resources << >> >>", []byte("1 1 1 rg 0 0 50 100 re f")),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	path := dl.Items[0].(*displaylist.Path)
	if path.SoftMask == nil || path.SoftMask.Subtype != "Alpha" || len(path.SoftMask.Items) == 0 {
		t.Fatalf("soft mask %#v", path.SoftMask)
	}
	if len(dl.Items) != 2 {
		t.Fatalf("soft mask items %d", len(dl.Items))
	}
	if dl.Items[1].(*displaylist.Path).SoftMask != path.SoftMask {
		t.Fatal("soft mask was not cached")
	}
}

func TestAdvancedColorSpaces(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /ColorSpace << /CS1 [/Separation /Spot /DeviceGray 6 0 R] /CS2 [/Indexed /DeviceRGB 1 <FF000000FF00>] /CS3 [/DeviceN [/R /G /B] /DeviceRGB 7 0 R] >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/CS1 cs 0.5 scn 0 0 10 10 re f /CS2 cs 1 scn 0 0 10 10 re f /CS3 cs 1 0 0 scn 0 0 10 10 re f")),
		testkit.Raw(6, "<< /FunctionType 2 /Domain [0 1] /C0 [0] /C1 [1] /N 1 >>"),
		testkit.Stream(7, "<< /FunctionType 4 /Domain [0 1 0 1 0 1] /Range [0 1 0 1 0 1] >>", nil),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dl.Items) != 3 {
		t.Fatalf("items %d", len(dl.Items))
	}
	sep := dl.Items[0].(*displaylist.Path).Fill
	if sep.Space != "Separation" || math.Abs(sep.RGBA[0]-0.5) > 1e-9 {
		t.Fatalf("separation %#v", sep)
	}
	idx := dl.Items[1].(*displaylist.Path).Fill
	if idx.Space != "Indexed" || idx.RGBA[0] != 0 || idx.RGBA[1] != 1 {
		t.Fatalf("indexed %#v", idx)
	}
	devn := dl.Items[2].(*displaylist.Path).Fill
	if devn.Space != "DeviceN" || devn.RGBA[0] != 1 || devn.RGBA[1] != 0 {
		t.Fatalf("devicen %#v", devn)
	}
}

func TestShading(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Shading << /Sh1 6 0 R /Sh2 7 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/Sh1 sh /Sh2 sh")),
		testkit.Raw(6, "<< /ShadingType 2 /ColorSpace /DeviceRGB /Coords [0 0 100 0] /Function 8 0 R /Extend [true false] >>"),
		testkit.Raw(7, "<< /ShadingType 3 /ColorSpace /DeviceRGB /Coords [50 50 0 50 50 50] /Function 9 0 R /Extend [false true] >>"),
		testkit.Raw(8, "<< /FunctionType 2 /Domain [0 1] /C0 [0 0 0] /C1 [1 0 0] /N 1 >>"),
		testkit.Raw(9, "<< /FunctionType 2 /Domain [0 1] /C0 [1 0 0] /C1 [0 0 1] /N 1 >>"),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dl.Items) != 2 {
		t.Fatalf("items %d", len(dl.Items))
	}
	axial := dl.Items[0].(*displaylist.Shading)
	if axial.Kind != displaylist.ShadingAxial || axial.Extend != [2]bool{true, false} {
		t.Fatalf("axial %#v", axial)
	}
	g := axial.Geometry.(displaylist.ShadingGeometry)
	if g.X0 != 0 || g.Y0 != 100 || g.X1 != 100 || g.Y1 != 100 {
		t.Fatalf("axial geometry %#v", g)
	}
	if len(axial.Stops) < 2 || axial.Stops[0].Offset != 0 || axial.Stops[0].Color[0] != 0 || axial.Stops[len(axial.Stops)-1].Color[0] != 1 {
		t.Fatalf("axial stops %#v", axial.Stops)
	}
	radial := dl.Items[1].(*displaylist.Shading)
	if radial.Kind != displaylist.ShadingRadial || radial.Extend != [2]bool{false, true} {
		t.Fatalf("radial %#v", radial)
	}
	rg := radial.Geometry.(displaylist.ShadingGeometry)
	if rg.R0 != 0 || rg.R1 != 50 || rg.X0 != 50 || rg.Y0 != 50 || rg.X1 != 50 || rg.Y1 != 50 {
		t.Fatalf("radial geometry %#v", rg)
	}
	if radial.Stops[0].Color[0] != 1 || radial.Stops[len(radial.Stops)-1].Color[2] != 1 {
		t.Fatalf("radial stops %#v", radial.Stops)
	}
}

func TestOptionalContentVisibility(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R /OCProperties << /OCGs [6 0 R] /D << /BaseState /ON /OFF [6 0 R] >> >> >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F1 7 0 R >> /Properties << /MC0 6 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/OC /MC0 BDC BT /F1 10 Tf 0 0 Td (Hidden) Tj ET EMC BT /F1 10 Tf 0 50 Td (Shown) Tj ET")),
		testkit.Raw(6, "<< /Type /OCG /Name (Layer) >>"),
		testkit.Raw(7, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"),
	}
	text := func(d *Document) string {
		t.Helper()
		defer d.Close()
		p, _ := d.Page(0)
		dl, err := p.DisplayList(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		for _, it := range dl.Items {
			if tr, ok := it.(*displaylist.TextRun); ok {
				for _, g := range tr.Glyphs {
					b.WriteString(g.Text)
				}
			}
		}
		return b.String()
	}
	if got := text(openTest(t, testkit.Build(objs, "/Root 1 0 R"))); got != "Shown" {
		t.Fatalf("default text %q", got)
	}
	if got := text(openTest(t, testkit.Build(objs, "/Root 1 0 R"), WithHiddenLayers(true))); got != "HiddenShown" {
		t.Fatalf("hidden-layer text %q", got)
	}
}

func TestPageScanned(t *testing.T) {
	imageObjs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /XObject << /Im1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("q 200 0 0 100 0 0 cm /Im1 Do Q")),
		testkit.Stream(5, "<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /BitsPerComponent 8 /ColorSpace /DeviceGray >>", []byte{128}),
	}
	d := openTest(t, testkit.Build(imageObjs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !dl.Scanned {
		t.Fatal("full-page image not marked scanned")
	}
}

func TestPageNotScannedWithText(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /XObject << /Im1 5 0 R >> /Font << /F1 6 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("q 200 0 0 100 0 0 cm /Im1 Do Q BT /F1 10 Tf 10 20 Td (Hi) Tj ET")),
		testkit.Stream(5, "<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /BitsPerComponent 8 /ColorSpace /DeviceGray >>", []byte{128}),
		testkit.Raw(6, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if dl.Scanned {
		t.Fatal("image page with text marked scanned")
	}
}

func TestSourceAndSignature(t *testing.T) {
	d := contentDoc(t, "0 0 10 10 re f 50 50 10 10 re f")
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dl.Items) != 2 {
		t.Fatalf("items %d", len(dl.Items))
	}
	a := dl.Items[0].(*displaylist.Path)
	b := dl.Items[1].(*displaylist.Path)
	if a.Hint != displaylist.PathRect || b.Hint != displaylist.PathRect {
		t.Fatalf("hints %d %d", a.Hint, b.Hint)
	}
	if a.Sig == 0 || a.Sig != b.Sig {
		t.Fatalf("signatures %d %d", a.Sig, b.Sig)
	}
	if a.Source.OpIndex == b.Source.OpIndex {
		t.Fatalf("source op indexes equal: %d", a.Source.OpIndex)
	}
}

func TestActualText(t *testing.T) {
	d := contentDoc(t, "/Span << /ActualText (Hi) >> BDC BT /F1 10 Tf 10 20 Td (Hi) Tj ET EMC")
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	tr := dl.Items[0].(*displaylist.TextRun)
	if tr.ActualText != "Hi" {
		t.Fatalf("actual text %q", tr.ActualText)
	}
	for _, dg := range dl.Diag {
		if dg.Code == "CNT-ACTUALTEXT-CONFLICT" {
			t.Fatalf("unexpected conflict: %#v", dl.Diag)
		}
	}
}

func TestActualTextConflict(t *testing.T) {
	d := contentDoc(t, "/Span << /ActualText (Hello) >> BDC BT /F1 10 Tf 10 20 Td (Hi) Tj ET EMC")
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	tr := dl.Items[0].(*displaylist.TextRun)
	if tr.ActualText != "Hello" {
		t.Fatalf("actual text %q", tr.ActualText)
	}
	found := false
	for _, dg := range dl.Diag {
		if dg.Code == "CNT-ACTUALTEXT-CONFLICT" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing conflict diagnostic: %#v", dl.Diag)
	}
}

func TestActualTextMultipleRuns(t *testing.T) {
	d := contentDoc(t, "/Span << /ActualText (AB) >> BDC BT /F1 10 Tf 10 20 Td (A) Tj ET BT /F1 10 Tf 20 20 Td (B) Tj ET EMC")
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dl.Items) != 2 {
		t.Fatalf("items %d", len(dl.Items))
	}
	if dl.Items[0].(*displaylist.TextRun).ActualText != "AB" {
		t.Fatalf("first actual text %q", dl.Items[0].(*displaylist.TextRun).ActualText)
	}
	if dl.Items[1].(*displaylist.TextRun).ActualText != "" {
		t.Fatalf("second actual text %q", dl.Items[1].(*displaylist.TextRun).ActualText)
	}
	found := false
	for _, dg := range dl.Diag {
		if dg.Code == "CNT-ACTUALTEXT-MULTI" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing multi-run diagnostic: %#v", dl.Diag)
	}
}

func TestTilingPattern(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Pattern << /P1 6 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/Pattern cs /P1 scn 0 0 10 10 re f /P1 scn 0 20 10 10 re f")),
		testkit.Stream(6, "<< /Type /Pattern /PatternType 1 /PaintType 1 /TilingType 1 /BBox [0 0 10 10] /XStep 10 /YStep 10 /Resources << >> >>", []byte("0 0 5 5 re f")),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	path := dl.Items[0].(*displaylist.Path)
	if path.Fill == nil || path.Fill.Kind != displaylist.PaintPattern || path.Fill.Pattern == nil {
		t.Fatalf("fill %#v", path.Fill)
	}
	pat := path.Fill.Pattern
	if pat.Name != "P1" || pat.Type != 1 || pat.PaintType != 1 || pat.XStep != 10 || pat.YStep != 10 || len(pat.Content) == 0 || len(pat.Items) == 0 {
		t.Fatalf("pattern %#v", pat)
	}
	if len(dl.Items) != 2 {
		t.Fatalf("pattern items %d", len(dl.Items))
	}
	pat2 := dl.Items[1].(*displaylist.Path).Fill.Pattern
	if pat2 == nil || &pat2.Items[0] != &pat.Items[0] {
		t.Fatal("pattern content was not cached")
	}
}

func TestIndirectPatternColorSpace(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /ColorSpace << /CS0 8 0 R >> /Pattern << /P1 6 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/CS0 cs /P1 scn 0 0 10 10 re f")),
		testkit.Stream(6, "<< /Type /Pattern /PatternType 1 /PaintType 1 /TilingType 1 /BBox [0 0 10 10] /XStep 10 /YStep 10 /Resources << >> >>", []byte("0 0 5 5 re f")),
		testkit.Raw(8, "[/Pattern]"),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dl.Items) != 1 {
		t.Fatalf("items %d", len(dl.Items))
	}
	path := dl.Items[0].(*displaylist.Path)
	if path.Fill == nil || path.Fill.Kind != displaylist.PaintPattern || path.Fill.Pattern == nil {
		t.Fatalf("fill %#v", path.Fill)
	}
}

func TestMeshShadingPassthrough(t *testing.T) {
	raw := []byte{0, 1, 2, 3, 4, 5, 6, 7}
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Shading << /Sh1 6 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/Sh1 sh")),
		testkit.Stream(6, "<< /ShadingType 4 /ColorSpace /DeviceRGB /BitsPerCoordinate 8 /BitsPerComponent 8 /BitsPerFlag 2 /Decode [0 100 0 100 0 1 0 1 0 1] >>", raw),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sh := dl.Items[0].(*displaylist.Shading)
	if sh.Kind != displaylist.ShadingMesh || !bytes.Equal(sh.Raw, raw) {
		t.Fatalf("shading %#v", sh)
	}
}

func TestPreserveTransparencyGroup(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /XObject << /Fm1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/Fm1 Do")),
		testkit.Stream(5, "<< /Type /XObject /Subtype /Form /BBox [0 0 100 100] /Group << /S /Transparency /I true /CS /DeviceRGB /BC [0.2 0.3 0.4] >> >>", []byte("1 0 0 rg 0 0 100 100 re f")),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"), WithPreserveForms(true))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	g, ok := dl.Items[0].(*displaylist.Group)
	if !ok || len(g.Items) != 1 || g.Name != "Fm1" || !g.Isolated {
		t.Fatalf("group %#v", dl.Items[0])
	}
	if g.BlendColorSpace != "DeviceRGB" || g.Backdrop == nil || math.Abs(g.Backdrop.Color[0]-0.2) > 1e-6 {
		t.Fatalf("group blend/backdrop %#v", g)
	}
}

func TestPageText(t *testing.T) {
	d := contentDoc(t, "BT /F1 12 Tf 10 80 Td (Hello world) Tj ET")
	defer d.Close()
	p, _ := d.Page(0)
	tx, err := p.Text(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tx.Text != "Hello world" || len(tx.Lines) != 1 || len(tx.Words) != 2 {
		t.Fatalf("text %q lines=%d words=%d", tx.Text, len(tx.Lines), len(tx.Words))
	}
}

func TestPageTextLines(t *testing.T) {
	d := contentDoc(t, "BT /F1 12 Tf 10 80 Td (Hello) Tj ET BT /F1 12 Tf 10 60 Td (World) Tj ET")
	defer d.Close()
	p, _ := d.Page(0)
	tx, err := p.Text(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tx.Text != "Hello\nWorld" || len(tx.Lines) != 2 {
		t.Fatalf("text %q lines=%d", tx.Text, len(tx.Lines))
	}
}

func TestPageTextActualText(t *testing.T) {
	d := contentDoc(t, "/Span << /ActualText (Hi) >> BDC BT /F1 12 Tf 10 80 Td (Hi) Tj ET EMC")
	defer d.Close()
	p, _ := d.Page(0)
	tx, err := p.Text(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tx.Text != "Hi" {
		t.Fatalf("actual text %q", tx.Text)
	}
}

func TestPageLinks(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Annots [6 0 R 7 0 R] >>"),
		testkit.Raw(4, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] >>"),
		testkit.Raw(6, "<< /Type /Annot /Subtype /Link /Rect [10 20 30 40] /A << /S /URI /URI (https://example.test/) >> >>"),
		testkit.Raw(7, "<< /Type /Annot /Subtype /Link /Rect [50 20 80 40] /Dest [4 0 R /XYZ 0 0 0] >>"),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	links, err := p.Links(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 {
		t.Fatalf("links %#v", links)
	}
	if links[0].URI != "https://example.test/" || links[0].Rect.X0 != 10 || links[0].Rect.Y0 != 60 || links[0].Rect.Y1 != 80 {
		t.Fatalf("uri link %#v", links[0])
	}
	if links[1].Page != 1 {
		t.Fatalf("goto link %#v", links[1])
	}
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dl.Links) != 2 {
		t.Fatalf("display links %#v", dl.Links)
	}
}

func TestNamedDestinationAndOutlines(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R /Dests << /Intro [4 0 R /Fit] >> /Outlines 10 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Annots [6 0 R] >>"),
		testkit.Raw(4, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] >>"),
		testkit.Raw(6, "<< /Type /Annot /Subtype /Link /Rect [10 20 30 40] /Dest (Intro) >>"),
		testkit.Raw(8, "<< /Title (Chapter) /Dest [4 0 R /Fit] /First 9 0 R >>"),
		testkit.Raw(9, "<< /Title (Section) /Parent 8 0 R /Dest [3 0 R /Fit] >>"),
		testkit.Raw(10, "<< /Type /Outlines /First 8 0 R /Last 8 0 R /Count 2 >>"),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	links, err := p.Links(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 || links[0].Page != 1 {
		t.Fatalf("named link %#v", links)
	}
	outline, err := d.Outlines(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(outline) != 1 || outline[0].Title != "Chapter" || outline[0].Page != 1 || len(outline[0].Children) != 1 || outline[0].Children[0].Page != 0 {
		t.Fatalf("outline %#v", outline)
	}
}

func TestPageLabels(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R /PageLabels << /Nums [0 << /S /r /St 1 >> 2 << /S /D /P (A-) /St 1 >>] >> >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R 4 0 R 5 0 R] /Count 3 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>"),
		testkit.Raw(4, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>"),
		testkit.Raw(5, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>"),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	for i, want := range []string{"i", "ii", "A-1"} {
		p, _ := d.Page(i)
		got, err := p.Label(context.Background())
		if err != nil || got != want {
			t.Fatalf("page %d label %q err %v", i, got, err)
		}
		dl, err := p.DisplayList(context.Background())
		if err != nil || dl.Label != want {
			t.Fatalf("display label %q err %v", dl.Label, err)
		}
	}
	ranges, err := d.PageLabelRanges(context.Background())
	if err != nil || len(ranges) != 2 || ranges[0].Start != 0 || ranges[0].Style != "r" ||
		ranges[1].Start != 2 || ranges[1].Prefix != "A-" {
		t.Fatalf("ranges = %#v err=%v", ranges, err)
	}
}

func TestPageAnnotations(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Annots [5 0 R 6 0 R] >>"),
		testkit.Raw(5, "<< /Type /Annot /Subtype /Link /Rect [10 20 30 40] /A << /S /URI /URI (https://example.test/) >> >>"),
		testkit.Raw(6, "<< /Type /Annot /Subtype /Widget /Rect [10 10 90 30] /F 4 /T (name) /FT /Tx /V (value) >>"),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	annots, err := p.Annotations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(annots) != 2 || annots[0].URI != "https://example.test/" || annots[1].FieldName != "name" || annots[1].FieldValue != "value" || annots[1].FieldType != "Tx" {
		t.Fatalf("annotations %#v", annots)
	}
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dl.Annotations) != 2 {
		t.Fatalf("display annotations %#v", dl.Annotations)
	}
}

func TestFormFields(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [5 0 R 6 0 R] >> >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>"),
		testkit.Raw(5, "<< /T (name) /FT /Tx /V (Alice) /Kids [7 0 R] >>"),
		testkit.Raw(6, "<< /T (agree) /FT /Btn /V /Yes /Rect [10 10 20 20] /P 3 0 R >>"),
		testkit.Raw(7, "<< /Type /Annot /Subtype /Widget /Parent 5 0 R /Rect [30 30 90 50] /P 3 0 R >>"),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	fields, err := d.FormFields(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 2 {
		t.Fatalf("fields %#v", fields)
	}
	if fields[0].Name != "name" || fields[0].Type != "Tx" || fields[0].Value != "Alice" || fields[0].Page != 0 || fields[0].Rect.X0 != 30 {
		t.Fatalf("text field %#v", fields[0])
	}
	if fields[1].Name != "agree" || fields[1].Value != "Yes" || fields[1].Page != 0 {
		t.Fatalf("checkbox %#v", fields[1])
	}
}

func TestStructureTree(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 5 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>"),
		testkit.Raw(5, "<< /Type /StructTreeRoot /RoleMap << /MyPara /P >> /ClassMap << /Body 8 0 R >> /K [6 0 R] >>"),
		testkit.Raw(6, "<< /Type /StructElem /S /MyPara /C [/Body /Important] /A << /O /Layout /BBox [0 0 10 20] /Placement /Block /Width 5 >> /T (Para) /Alt (Alt para) /Lang (en-US) /ID (elem-1) /Pg 3 0 R /K [0 1 7 0 R] >>"),
		testkit.Raw(7, "<< /Type /StructElem /S /Span /Pg 3 0 R /K 2 >>"),
		testkit.Raw(8, "<< /O /Table /ColSpan 2 /Scope /Column >>"),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	tree, err := d.StructureTree(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 1 || tree[0].Type != "P" || tree[0].Title != "Para" || tree[0].Page != 0 {
		t.Fatalf("tree %#v", tree)
	}
	if tree[0].RawType != "MyPara" || tree[0].Alt != "Alt para" || tree[0].Lang != "en-US" || tree[0].IDName != "elem-1" {
		t.Fatalf("metadata %#v", tree[0])
	}
	if len(tree[0].Classes) != 2 || tree[0].Classes[0] != "Body" || tree[0].Classes[1] != "Important" {
		t.Fatalf("classes %#v", tree[0])
	}
	if len(tree[0].Attributes) != 1 || tree[0].Attributes[0].Owner != "Layout" {
		t.Fatalf("attributes %#v", tree[0].Attributes)
	}
	attrs := tree[0].Attributes[0].Values
	if attrs["Placement"].Kind != "name" || attrs["Placement"].Text != "Block" {
		t.Fatalf("placement %#v", attrs["Placement"])
	}
	if attrs["Width"].Kind != "number" || attrs["Width"].Number != 5 {
		t.Fatalf("width %#v", attrs["Width"])
	}
	if attrs["BBox"].Kind != "array" || len(attrs["BBox"].Items) != 4 || attrs["BBox"].Items[3].Number != 20 {
		t.Fatalf("bbox %#v", attrs["BBox"])
	}
	if len(tree[0].ClassAttributes) != 1 || tree[0].ClassAttributes[0].Owner != "Table" || tree[0].ClassAttributes[0].Values["ColSpan"].Number != 2 {
		t.Fatalf("class attributes %#v", tree[0].ClassAttributes)
	}
	if bbox, ok := tree[0].BBox(); !ok || bbox.X0 != 0 || bbox.Y1 != 20 {
		t.Fatalf("bbox %#v", bbox)
	}
	if placement, ok := tree[0].Placement(); !ok || placement != "Block" {
		t.Fatalf("placement %q", placement)
	}
	if span, ok := tree[0].ColSpan(); !ok || span != 2 {
		t.Fatalf("colspan %d", span)
	}
	if scope, ok := tree[0].Scope(); !ok || scope != "Column" {
		t.Fatalf("scope %q", scope)
	}
	if len(tree[0].MCIDs) != 2 || len(tree[0].Children) != 1 || tree[0].Children[0].Type != "Span" || tree[0].Children[0].MCIDs[0] != 2 {
		t.Fatalf("tree %#v", tree)
	}
}

func TestMarkedContentMCID(t *testing.T) {
	d := contentDoc(t, "/P << /MCID 7 >> BDC BT /F1 12 Tf 10 80 Td (Hi) Tj ET EMC")
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	tr := dl.Items[0].(*displaylist.TextRun)
	if tr.MCID != 7 {
		t.Fatalf("MCID %d", tr.MCID)
	}
}

func TestPageTextTaggedOrder(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 5 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /Font << /F1 8 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/P << /MCID 1 >> BDC BT /F1 12 Tf 10 70 Td (Alpha) Tj ET EMC /P << /MCID 2 >> BDC BT /F1 12 Tf 10 50 Td (Beta) Tj ET EMC")),
		testkit.Raw(5, "<< /Type /StructTreeRoot /K [7 0 R 6 0 R] >>"),
		testkit.Raw(6, "<< /Type /StructElem /S /P /T (First title) /Alt (First alt) /Lang (he-IL) /Pg 3 0 R /K 1 >>"),
		testkit.Raw(7, "<< /Type /StructElem /S /H1 /T (Second title) /Alt (Second alt) /Lang (en-US) /Pg 3 0 R /K 2 >>"),
		testkit.Raw(8, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	tx, err := p.Text(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tx.Text != "Beta\nAlpha" {
		t.Fatalf("tagged text = %q", tx.Text)
	}
	if len(tx.Blocks) != 2 || tx.Blocks[0].Type != "H1" || tx.Blocks[0].Alt != "Second alt" || tx.Blocks[0].Lang != "en-US" {
		t.Fatalf("blocks = %#v", tx.Blocks)
	}
	if tx.Blocks[1].Type != "P" || tx.Blocks[1].Title != "First title" || tx.Blocks[1].Lang != "he-IL" {
		t.Fatalf("blocks = %#v", tx.Blocks)
	}
}

func TestStructureTreeMCIDMapping(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 5 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /Font << /F1 7 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/P << /MCID 3 >> BDC BT /F1 12 Tf 10 50 Td (Hi) Tj ET EMC")),
		testkit.Raw(5, "<< /Type /StructTreeRoot /K [6 0 R] >>"),
		testkit.Raw(6, "<< /Type /StructElem /S /P /Pg 3 0 R /K 3 >>"),
		testkit.Raw(7, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	tr := dl.Items[0].(*displaylist.TextRun)
	if tr.MCID != 3 || tr.StructRef == 0 {
		t.Fatalf("mcid=%d struct=%d", tr.MCID, tr.StructRef)
	}
}

func TestAttachments(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R /Names << /EmbeddedFiles 5 0 R >> >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>"),
		testkit.Raw(5, "<< /Names [(file.txt) 6 0 R] >>"),
		testkit.Raw(6, "<< /Type /Filespec /F (file.txt) /UF (file.txt) /Desc (attached) /EF << /F 7 0 R >> >>"),
		testkit.Stream(7, "<< /Type /EmbeddedFile /Subtype /text /Params << /Size 5 /ModDate (D:20260101000000Z) >> >>", []byte("hello")),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	atts, err := d.Attachments(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(atts) != 1 || atts[0].Name != "file.txt" || atts[0].Description != "attached" || atts[0].Size != 5 {
		t.Fatalf("attachments %#v", atts)
	}
	data, err := atts[0].Data(context.Background())
	if err != nil || string(data) != "hello" {
		t.Fatalf("data %q %v", data, err)
	}
	var buf bytes.Buffer
	n, err := atts[0].WriteTo(context.Background(), &buf)
	if err != nil || n != 5 || buf.String() != "hello" {
		t.Fatalf("writeTo n=%d data=%q err=%v", n, buf.String(), err)
	}
}

func TestXMPMetadata(t *testing.T) {
	xmp := []byte(`<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
 <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
  <rdf:Description xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:pdf="http://ns.adobe.com/pdf/1.3/" xmlns:xmp="http://ns.adobe.com/xap/1.0/">
   <dc:title><rdf:Alt><rdf:li xml:lang="x-default">XMP Title</rdf:li></rdf:Alt></dc:title>
   <dc:creator><rdf:Seq><rdf:li>XMP Author</rdf:li><rdf:li>Second Author</rdf:li></rdf:Seq></dc:creator>
   <dc:description><rdf:Alt><rdf:li xml:lang="x-default">XMP Subject</rdf:li></rdf:Alt></dc:description>
   <dc:subject><rdf:Bag><rdf:li>one</rdf:li><rdf:li>two</rdf:li></rdf:Bag></dc:subject>
   <pdf:Producer>XMP Producer</pdf:Producer>
   <xmp:CreateDate>2026-01-02T03:04:05Z</xmp:CreateDate>
  </rdf:Description>
 </rdf:RDF>
</x:xmpmeta>`)
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R /Metadata 4 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>"),
		testkit.Stream(4, "<< /Type /Metadata /Subtype /XML >>", xmp),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	info := d.Info()
	if info.Title != "XMP Title" || info.Author != "XMP Author; Second Author" ||
		info.Subject != "XMP Subject" || info.Keywords != "one, two" ||
		info.Producer != "XMP Producer" || info.CreationDate != "2026-01-02T03:04:05Z" {
		t.Fatalf("info %#v", info)
	}
}

func TestPageSearch(t *testing.T) {
	d := contentDoc(t, "BT /F1 12 Tf 10 20 Td (Hello World) Tj ET")
	defer d.Close()
	p, _ := d.Page(0)
	hits, err := p.Search(context.Background(), "world", pdftext.SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Bounds.X0 <= 10 {
		t.Fatalf("hits %#v", hits)
	}
}

func TestVerticalCIDPositioning(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("BT /F1 100 Tf 10 100 Td <0001> Tj <0002> Tj ET")),
		testkit.Raw(5, "<< /Type /Font /Subtype /Type0 /BaseFont /SimSun /Encoding /Identity-V /DescendantFonts [6 0 R] >>"),
		testkit.Raw(6, "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /SimSun /DW 1000 /DW2 [880 -1000] /W [1 [500] 2 [500]] /W2 [1 [750 250 900] 2 [750 250 900]] >>"),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dl.Items) != 1 {
		t.Fatalf("items %d", len(dl.Items))
	}
	tr, ok := dl.Items[0].(*displaylist.TextRun)
	if !ok || !tr.Vertical || len(tr.Glyphs) != 2 {
		t.Fatalf("run %#v", dl.Items[0])
	}
	g0, g1 := tr.Glyphs[0], tr.Glyphs[1]
	if math.Abs(g0.Origin.X-35) > 0.01 || math.Abs(g0.Origin.Y-100) > 0.01 {
		t.Fatalf("g0 %+v", g0.Origin)
	}
	if math.Abs(g0.Advance-75) > 0.01 {
		t.Fatalf("advance %v", g0.Advance)
	}
	if math.Abs(g1.Origin.X-35) > 0.01 || math.Abs(g1.Origin.Y-175) > 0.01 {
		t.Fatalf("g1 %+v", g1.Origin)
	}
}

func TestTextSpanAndPaintState(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << /Font << /F1 5 0 R >> /ExtGState << /GS1 6 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/Span << /ActualText (AB) >> BDC q /GS1 gs BT /F1 10 Tf 10 20 Td (AB) Tj ET 0 0 10 10 re f Q EMC")),
		testkit.Raw(5, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"),
		testkit.Raw(6, "<< /Type /ExtGState /ca 0.5 /CA 0.25 /OP true /op false /OPM 1 /RI /Perceptual >>"),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dl.Items) != 2 {
		t.Fatalf("items %d", len(dl.Items))
	}
	tr := dl.Items[0].(*displaylist.TextRun)
	if len(tr.Spans) != 1 || tr.Spans[0].Text != "AB" || tr.Spans[0].GlyphEnd != 1 {
		t.Fatalf("spans %#v", tr.Spans)
	}
	ps := tr.PaintState
	if ps == nil || math.Abs(ps.FillAlpha-0.5) > 1e-9 || math.Abs(ps.StrokeAlpha-0.25) > 1e-9 {
		t.Fatalf("text paint state %#v", ps)
	}
	path := dl.Items[1].(*displaylist.Path)
	ps = path.PaintState
	if ps == nil || math.Abs(ps.FillAlpha-0.5) > 1e-9 || math.Abs(ps.StrokeAlpha-0.25) > 1e-9 {
		t.Fatalf("path paint state %#v", ps)
	}
}

func TestPatternCacheBudget(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Pattern << /P1 6 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/Pattern cs /P1 scn 0 0 10 10 re f /P1 scn 0 20 10 10 re f")),
		testkit.Stream(6, "<< /Type /Pattern /PatternType 1 /PaintType 1 /TilingType 1 /BBox [0 0 10 10] /XStep 10 /YStep 10 /Resources << >> >>", []byte("0 0 5 5 re f")),
	}
	data := testkit.Build(objs, "/Root 1 0 R")
	b := DefaultBudget
	b.MaxLiveBytes = int64(len(data)) + 100
	d := openTest(t, data, WithBudget(&b))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pat2 := dl.Items[1].(*displaylist.Path).Fill.Pattern
	found := false
	for _, dg := range d.Diagnostics() {
		if dg.Code == "BUD-PATTERN-CACHE" {
			found = true
		}
	}
	if !found || pat2 == nil {
		t.Fatalf("diag=%v pattern=%v", found, pat2)
	}
}

type testColorManager struct {
	space   string
	profile string
	values  []float64
}

func (m *testColorManager) Convert(info ColorSpaceInfo, values []float64) ([4]float64, bool, error) {
	m.space = info.Name
	m.profile = string(info.ICCProfile)
	m.values = append([]float64(nil), values...)
	if info.Name == "DeviceCMYK" {
		return [4]float64{0, 1, 0, 1}, false, nil
	}
	return [4]float64{0, 0, 1, 1}, false, nil
}

func TestColorManagerCMYK(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("1 0 0 0 k 0 0 10 10 re f")),
	}
	m := &testColorManager{}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"), WithColorManager(m))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	path := dl.Items[0].(*displaylist.Path)
	if m.space != "DeviceCMYK" || len(m.values) != 4 || path.Fill.RGBA != [4]float64{0, 1, 0, 1} {
		t.Fatalf("manager=%#v fill=%v", m, path.Fill.RGBA)
	}
}

func TestColorManagerICC(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /ColorSpace << /CS1 [/ICCBased 5 0 R] >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/CS1 cs 1 0 0 scn 0 0 10 10 re f")),
		testkit.Stream(5, "<< /N 3 >>", []byte("profile")),
	}
	m := &testColorManager{}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"), WithColorManager(m))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	path := dl.Items[0].(*displaylist.Path)
	if m.space != "ICCBased" || m.profile != "profile" || path.Fill.RGBA != [4]float64{0, 0, 1, 1} {
		t.Fatalf("manager=%#v fill=%v", m, path.Fill.RGBA)
	}
}

func TestOptionalContentLayerIdentity(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R /OCProperties << /OCGs [6 0 R] /D << /OFF [6 0 R] >> >> >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /Properties << /OC1 6 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/OC /OC1 BDC 1 0 0 rg 0 0 10 10 re f EMC")),
		testkit.Raw(6, "<< /Type /OCG /Name (Layer1) >>"),
	}
	data := testkit.Build(objs, "/Root 1 0 R")
	d := openTest(t, data)
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dl.Items) != 0 {
		t.Fatalf("hidden layer emitted %d items", len(dl.Items))
	}
	_ = d.Close()

	d2 := openTest(t, data, WithHiddenLayers(true))
	defer d2.Close()
	p2, _ := d2.Page(0)
	dl2, err := p2.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dl2.Items) != 1 || dl2.Items[0].Base().Layer != "Layer1" {
		t.Fatalf("layer %#v", dl2.Items)
	}
}

func TestOCMDVisibility(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R /OCProperties << /OCGs [6 0 R 7 0 R] /D << /BaseState /ON /OFF [7 0 R] >> >> >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /Properties << /Any 8 0 R /All 9 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/OC /Any BDC 1 0 0 rg 0 0 10 10 re f EMC /OC /All BDC 0 1 0 rg 0 0 10 10 re f EMC")),
		testkit.Raw(6, "<< /Type /OCG /Name (On) >>"),
		testkit.Raw(7, "<< /Type /OCG /Name (Off) >>"),
		testkit.Raw(8, "<< /Type /OCMD /OCGs [6 0 R 7 0 R] /P /AnyOn >>"),
		testkit.Raw(9, "<< /Type /OCMD /VE [/And 6 0 R 7 0 R] >>"),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dl.Items) != 1 {
		t.Fatalf("items %d", len(dl.Items))
	}
}

func TestType3SubsceneText(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /Font << /F3 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("BT /F3 100 Tf 10 10 Td (A) Tj ET")),
		testkit.Raw(5, "<< /Type /Font /Subtype /Type3 /FontBBox [0 0 1000 1000] /FontMatrix [0.001 0 0 0.001 0 0] /CharProcs << /A 7 0 R >> /Encoding << /Type /Encoding /Differences [65 /A] >> /FirstChar 65 /LastChar 65 /Widths [600] /Resources << /Font << /F1 8 0 R >> >> >>"),
		testkit.Stream(7, "", []byte("BT /F1 10 Tf 100 100 Td (Hi) Tj ET")),
		testkit.Raw(8, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range dl.Items {
		tr, ok := it.(*displaylist.TextRun)
		if !ok {
			continue
		}
		if tr.Flags&displaylist.FlagFromType3 == 0 {
			continue
		}
		if len(tr.Glyphs) == 2 && tr.Glyphs[0].Text == "H" && tr.Glyphs[1].Text == "i" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Type3 text subscene not emitted: %#v", dl.Items)
	}
	tp, err := p.Text(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tp.Text != "A" {
		t.Fatalf("semantic text %q", tp.Text)
	}
}

func TestSoftMaskBackdrop(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /ExtGState << /GS1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("q /GS1 gs 1 0 0 rg 0 0 100 100 re f Q")),
		testkit.Raw(5, "<< /Type /ExtGState /SMask << /S /Luminosity /G 6 0 R /BC [0.5] >> >>"),
		testkit.Stream(6, "<< /Type /XObject /Subtype /Form /BBox [0 0 100 100] /Group << /S /Transparency /CS /DeviceGray >> >>", []byte("0.5 g 0 0 50 100 re f")),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	path := dl.Items[0].(*displaylist.Path)
	sm := path.SoftMask
	if sm == nil || !sm.HasBackdrop || math.Abs(sm.Backdrop[0]-0.5) > 1e-6 {
		t.Fatalf("soft mask %#v", sm)
	}
}

func TestGroupICCBlendSpaceName(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /XObject << /Fm1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/Fm1 Do")),
		testkit.Stream(5, "<< /Type /XObject /Subtype /Form /BBox [0 0 100 100] /Group << /S /Transparency /I true /CS [/ICCBased 7 0 R] >> >>", []byte("1 0 0 rg 0 0 100 100 re f")),
		testkit.Stream(7, "<< /N 4 >>", []byte("profile")),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"), WithPreserveForms(true))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	g, ok := dl.Items[0].(*displaylist.Group)
	if !ok || g.BlendColorSpace != "ICCBased" {
		t.Fatalf("group %#v", g)
	}
}

type fakeExtensionFactory struct{}

func (fakeExtensionFactory) Info() extension.ProviderInfo {
	return extension.ProviderInfo{Name: "test-extension", Version: "1", License: "MIT", Determinism: extension.Deterministic}
}
func (fakeExtensionFactory) Features() []extension.Feature { return nil }
func (fakeExtensionFactory) Encodings() []extension.ImageEncoding {
	return []extension.ImageEncoding{extension.ImageJBIG2}
}
func (fakeExtensionFactory) NewDecoder(context.Context, extension.ImageConfig) (extension.ImageDecoder, error) {
	return fakeExtensionDecoder{}, nil
}

type fakeExtensionDecoder struct{}

func (fakeExtensionDecoder) Decode(context.Context, io.ReaderAt, int64, extension.ImageConfig) (*extension.ImagePlane, error) {
	return &extension.ImagePlane{
		Pix: []byte{77}, Stride: 1, Width: 1, Height: 1,
		Components: 1, BitsPerComponent: 8, ColorSpace: "DeviceGray",
	}, nil
}
func (fakeExtensionDecoder) Close() error { return nil }

func TestExternalCCITTG32D(t *testing.T) {
	dec := &fakeJBIG2Decoder{}
	factory := fakeEncodingFactory{dec: dec, enc: "CCITTFaxDecode"}
	reg, err := NewDecoderRegistry(factory)
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte{1, 2, 3}
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /XObject << /Im1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/Im1 Do")),
		testkit.Stream(5, "<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /BitsPerComponent 1 /ColorSpace /DeviceGray /Filter /CCITTFaxDecode /DecodeParms << /K 1 /Columns 1 /Rows 1 >> >>", raw),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"), WithDecoderRegistry(reg))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	im := dl.Items[0].(*displaylist.Image)
	if _, err := im.Data.Decode(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(dec.raw, raw) {
		t.Fatalf("provider raw = %v", dec.raw)
	}
}

type fakeEncodingFactory struct {
	dec *fakeJBIG2Decoder
	enc displaylist.ImageEncoding
}

func (f fakeEncodingFactory) Encodings() []displaylist.ImageEncoding {
	return []displaylist.ImageEncoding{f.enc}
}
func (f fakeEncodingFactory) NewDecoder(context.Context, displaylist.ImageInfo) (ExternalDecoder, error) {
	return f.dec, nil
}

func TestExtensionImageDecoder(t *testing.T) {
	extReg, err := extension.NewRegistry(extension.WithImageFactory(fakeExtensionFactory{}))
	if err != nil {
		t.Fatal(err)
	}
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /XObject << /Im1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/Im1 Do")),
		testkit.Stream(5, "<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /BitsPerComponent 1 /ColorSpace /DeviceGray /Filter /JBIG2Decode >>", []byte{9, 8, 7}),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"), WithExtensionRegistry(extReg))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	im := dl.Items[0].(*displaylist.Image)
	img, err := im.Data.Decode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	gray, ok := img.(*stdimage.Gray)
	if !ok || gray.GrayAt(0, 0).Y != 77 {
		t.Fatalf("extension pixel = %#v", img)
	}
	sampler, ok := im.Data.(displaylist.ImageSampleRef)
	if !ok {
		t.Fatal("image does not expose source samples")
	}
	samples, err := sampler.DecodeSamples(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer samples.Close()
	if samples.Width != 1 || samples.Height != 1 || samples.Components != 1 || samples.ColorSpace != "DeviceGray" {
		t.Fatalf("samples = %#v", samples)
	}
	if got := samples.Data[0]; got < 0.301 || got > 0.303 {
		t.Fatalf("sample = %v", got)
	}
}

func TestExtensionImageDecoderConflict(t *testing.T) {
	extReg, err := extension.NewRegistry(extension.WithImageFactory(fakeExtensionFactory{}))
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := NewDecoderRegistry(fakeJBIG2Factory{dec: &fakeJBIG2Decoder{}})
	if err != nil {
		t.Fatal(err)
	}
	data := testkit.Build([]testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] >>"),
	}, "/Root 1 0 R")
	if _, err := Open(context.Background(), bytes.NewReader(data), int64(len(data)), WithDecoderRegistry(legacy), WithExtensionRegistry(extReg)); err == nil {
		t.Fatal("expected duplicate provider error")
	}
}

func TestImagePlaneToImageCMYK(t *testing.T) {
	img, err := imagePlaneToImage(&extension.ImagePlane{
		Pix: []byte{0, 255, 0, 0}, Stride: 4, Width: 1, Height: 1,
		Components: 4, BitsPerComponent: 8, ColorSpace: "DeviceCMYK",
	})
	if err != nil {
		t.Fatal(err)
	}
	cmyk, ok := img.(*stdimage.CMYK)
	if !ok || cmyk.CMYKAt(0, 0).M != 255 {
		t.Fatalf("cmyk = %#v", img)
	}
}

func TestColorManagerIndexedAndTintSpaces(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /ColorSpace << /IDX [/Indexed /DeviceCMYK 0 <00000000>] /SEP [/Separation /Spot /DeviceCMYK 5 0 R] /DEV [/DeviceN [/A /B] /DeviceCMYK 6 0 R] >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/IDX cs 0 scn 0 0 10 10 re f /SEP cs 0.5 scn 10 0 10 10 re f /DEV cs 0.5 0.5 scn 20 0 10 10 re f")),
		testkit.Stream(5, "<< /FunctionType 2 /Domain [0 1] /C0 [0 0 0 0] /C1 [1 1 1 1] /N 1 >>", nil),
		testkit.Stream(6, "<< /FunctionType 4 /Domain [0 1 0 1] /Range [0 1 0 1 0 1 0 1] >>", []byte("{ pop pop 0 0 0 0 }")),
	}
	m := &testColorManager{}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"), WithColorManager(m))
	defer d.Close()
	p, _ := d.Page(0)
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dl.Items) != 3 {
		t.Fatalf("items = %d", len(dl.Items))
	}
	for _, it := range dl.Items {
		path := it.(*displaylist.Path)
		if path.Fill.RGBA != [4]float64{0, 1, 0, 1} {
			t.Fatalf("fill = %v", path.Fill.RGBA)
		}
	}
	if m.space != "DeviceCMYK" {
		t.Fatalf("manager space = %q", m.space)
	}
}
