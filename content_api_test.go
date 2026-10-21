package pdf

import (
	"context"
	"fmt"
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
