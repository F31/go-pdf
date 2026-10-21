package pdf

import (
	"bytes"
	"context"
	"fmt"
	stdimage "image"
	stdcolor "image/color"
	"image/jpeg"
	"math"
	"strings"
	"testing"

	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/internal/testkit"
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
	testkit.Golden(t, "testdata/goldens/displaylist-text.json", b)
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

func (d *fakeJBIG2Decoder) CanDecode(enc displaylist.ImageEncoding) bool {
	return enc == "JBIG2Decode"
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

func TestExternalImageDecoder(t *testing.T) {
	dec := &fakeJBIG2Decoder{}
	RegisterExternalDecoder(dec)
	raw := []byte{9, 8, 7}
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /XObject << /Im1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/Im1 Do")),
		testkit.Stream(5, "<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /BitsPerComponent 1 /ColorSpace /DeviceGray /Filter /JBIG2Decode >>", raw),
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
		testkit.Stream(4, "", []byte("/GS1 gs 1 0 0 rg 0 0 100 100 re f")),
		testkit.Raw(5, "<< /Type /ExtGState /SMask 6 0 R >>"),
		testkit.Stream(6, "<< /Type /XObject /Subtype /Form /BBox [0 0 100 100] /Group << /S /Transparency /CS /DeviceGray >> >>", []byte("1 1 1 rg 0 0 50 100 re f")),
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
		testkit.Stream(4, "", []byte("/Pattern cs /P1 scn 0 0 10 10 re f")),
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
		testkit.Stream(5, "<< /Type /XObject /Subtype /Form /BBox [0 0 100 100] /Group << /S /Transparency /I true >> >>", []byte("1 0 0 rg 0 0 100 100 re f")),
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
}
