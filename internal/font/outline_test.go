package font

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"

	xfont "golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/internal/syntax"
	"github.com/F31/go-pdf/internal/testkit"
)

func loadSimpleOutlineTestFont(t *testing.T) *Font {
	t.Helper()
	objs := []testkit.Obj{
		testkit.Raw(5, "<< /Type /Font /Subtype /TrueType /BaseFont /GoPDFTestSans /Encoding /WinAnsiEncoding /FontDescriptor 6 0 R >>"),
		testkit.Raw(6, "<< /Type /FontDescriptor /FontName /GoPDFTestSans /Flags 32 /FontBBox [0 0 1000 1000] /ItalicAngle 0 /Ascent 905 /Descent -212 /CapHeight 700 /StemV 80 /FontFile2 7 0 R >>"),
		testkit.Stream(7, "", testFontData(t)),
	}
	file, dc := type0FileWith(t, objs)
	t.Cleanup(func() { _ = file.Close() })
	o, _ := file.ResolveRef(context.Background(), syntax.Ref{Num: 5})
	f, err := Load(context.Background(), file, o.(syntax.Dict), dc)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestGlyphOutlineSimple(t *testing.T) {
	f := loadSimpleOutlineTestFont(t)
	seg, ok := f.GlyphOutline(Code{Value: 'H', Bytes: 1})
	if !ok || len(seg) == 0 {
		t.Fatalf("outline ok=%v segments=%d", ok, len(seg))
	}
	if seg[0].Kind != 0 || seg[len(seg)-1].Kind != 3 {
		t.Fatalf("outline does not start with move and end with close: %#v", seg)
	}
	b := outlineBounds(seg)
	if math.Abs(b.X0-82) > 2 || math.Abs(b.X1-641) > 2 || math.Abs(b.Y0) > 2 || math.Abs(b.Y1-688) > 2 {
		t.Fatalf("bounds %#v", b)
	}

	var buf sfnt.Buffer
	gid, err := f.program.face.GlyphIndex(&buf, 'H')
	if err != nil {
		t.Fatal(err)
	}
	gb, _, err := f.program.face.GlyphBounds(&buf, gid, fixed.I(f.program.upem), xfont.HintingNone)
	if err != nil {
		t.Fatal(err)
	}
	scale := 1000 / float64(f.program.upem)
	want := [4]float64{
		float64(gb.Min.X) / 64 * scale,
		-float64(gb.Max.Y) / 64 * scale,
		float64(gb.Max.X) / 64 * scale,
		-float64(gb.Min.Y) / 64 * scale,
	}
	got := [4]float64{b.X0, b.Y0, b.X1, b.Y1}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1 {
			t.Fatalf("bounds %v, want %v", got, want)
		}
	}

	if _, ok := f.GlyphOutline(Code{Value: 'A', Bytes: 1}); ok {
		t.Fatal("unmapped code produced an outline")
	}
}

func TestGlyphOutlineCID(t *testing.T) {
	fontData := testFontData(t)
	cmap := "begincmap\n1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n4 beginbfchar\n<0001> <0048>\n<0002> <0065>\n<0003> <006C>\n<0004> <006F>\nendbfchar\nendcmap\n"
	cidToGID := []byte{0, 0, 0, 1, 0, 2, 0, 3, 0, 4}
	objs := []testkit.Obj{
		testkit.Raw(5, "<< /Type /Font /Subtype /Type0 /BaseFont /GoPDFTestSans /Encoding /Identity-H /DescendantFonts [6 0 R] /ToUnicode 8 0 R >>"),
		testkit.Raw(6, "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /GoPDFTestSans /DW 1000 /W [1 [722 556 222 556]] /CIDToGIDMap 9 0 R /FontDescriptor 7 0 R >>"),
		testkit.Raw(7, "<< /Type /FontDescriptor /FontName /GoPDFTestSans /Flags 32 /FontBBox [0 0 1000 1000] /ItalicAngle 0 /Ascent 905 /Descent -212 /CapHeight 700 /StemV 80 /FontFile2 10 0 R >>"),
		testkit.Stream(8, "", []byte(cmap)),
		testkit.Stream(9, "", cidToGID),
		testkit.Stream(10, "", fontData),
	}
	file, dc := type0FileWith(t, objs)
	defer file.Close()
	o, _ := file.ResolveRef(context.Background(), syntax.Ref{Num: 5})
	f, err := Load(context.Background(), file, o.(syntax.Dict), dc)
	if err != nil {
		t.Fatal(err)
	}
	cid, ok := f.GlyphOutline(Code{Value: 1, Bytes: 2, CID: 1})
	if !ok || len(cid) == 0 {
		t.Fatalf("CID outline ok=%v segments=%d", ok, len(cid))
	}
	simple, ok := loadSimpleOutlineTestFont(t).GlyphOutline(Code{Value: 'H', Bytes: 1})
	if !ok || len(simple) == 0 {
		t.Fatalf("simple outline ok=%v segments=%d", ok, len(simple))
	}
	if len(cid) != len(simple) {
		t.Fatalf("CID outline segments %d, simple %d", len(cid), len(simple))
	}
	for i := range cid {
		if cid[i] != simple[i] {
			t.Fatalf("segment %d differs:\n%#v\n%#v", i, cid[i], simple[i])
		}
	}
}

func TestGlyphOutlineUnavailable(t *testing.T) {
	if _, ok := (&Font{}).GlyphOutline(Code{Value: 'A', Bytes: 1}); ok {
		t.Fatal("empty font produced an outline")
	}
	f := &Font{type3: true, program: &fontProgram{}}
	if _, ok := f.GlyphOutline(Code{Value: 1, Bytes: 1}); ok {
		t.Fatal("Type3 font produced an sfnt outline")
	}
}

func TestGlyphOutlineGolden(t *testing.T) {
	f := loadSimpleOutlineTestFont(t)
	for _, tc := range []struct {
		code  byte
		path  string
		cubic bool
	}{
		{'H', "testdata/glyph-outline-H.txt", false},
		{'e', "testdata/glyph-outline-e.txt", true},
	} {
		seg, ok := f.GlyphOutline(Code{Value: uint32(tc.code), Bytes: 1})
		if !ok {
			t.Fatalf("%q: no outline", tc.code)
		}
		cubic := false
		for _, s := range seg {
			if s.Kind == displaylist.SegmentCubic {
				cubic = true
				break
			}
		}
		if cubic != tc.cubic {
			t.Fatalf("%q: cubic=%v want %v", tc.code, cubic, tc.cubic)
		}
		testkit.Golden(t, tc.path, formatOutline(seg))
	}
}

func formatOutline(seg []displaylist.Segment) []byte {
	var b strings.Builder
	for _, s := range seg {
		fmt.Fprintf(&b, "%d %.4f %.4f %.4f %.4f %.4f %.4f %.4f %.4f\n",
			s.Kind, s.P0.X, s.P0.Y, s.P1.X, s.P1.Y, s.P2.X, s.P2.Y, s.P3.X, s.P3.Y)
	}
	return []byte(b.String())
}
