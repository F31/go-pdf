package font

import (
	"context"
	"testing"

	"github.com/F31/go-pdf/internal/budget"
	"github.com/F31/go-pdf/internal/diag"
	"github.com/F31/go-pdf/internal/syntax"
	"github.com/F31/go-pdf/internal/testkit"
)

func type0File(t *testing.T) (*syntax.File, *diag.Collector) {
	t.Helper()
	objs := []testkit.Obj{
		testkit.Raw(5, "<< /Type /Font /Subtype /Type0 /BaseFont /SimSun /Encoding /Identity-H /DescendantFonts [6 0 R] /ToUnicode 7 0 R >>"),
		testkit.Raw(6, "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /SimSun /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /DW 1000 /W [1 [500 600] 16 18 700] >>"),
		testkit.Stream(7, "", []byte(sampleCMap)),
	}
	b := budget.DefaultBudget
	dc := diag.New(100)
	f, err := syntax.Open(context.Background(), testkit.Build(objs, "/Root 5 0 R"), syntax.Options{Tracker: budget.NewTracker(&b), Diag: dc})
	if err != nil {
		t.Fatal(err)
	}
	return f, dc
}

func TestType0Identity(t *testing.T) {
	file, dc := type0File(t)
	defer file.Close()
	o, err := file.ResolveRef(context.Background(), syntax.Ref{Num: 5})
	if err != nil {
		t.Fatal(err)
	}
	dict := o.(syntax.Dict)
	f, err := Load(context.Background(), file, dict, dc)
	if err != nil {
		t.Fatal(err)
	}
	if !f.isCID || f.CodeBytes() != 2 {
		t.Fatalf("cid=%v bytes=%d", f.isCID, f.CodeBytes())
	}
	codes := f.Codes([]byte{0, 1, 0, 2, 0, 16})
	if len(codes) != 3 {
		t.Fatalf("codes %d", len(codes))
	}
	if codes[0].CID != 1 || codes[0].Value != 1 {
		t.Fatalf("code0 %#v", codes[0])
	}
	if f.WidthCode(codes[0]) != 500 || f.WidthCode(codes[1]) != 600 || f.WidthCode(codes[2]) != 700 {
		t.Fatalf("widths %v %v %v", f.WidthCode(codes[0]), f.WidthCode(codes[1]), f.WidthCode(codes[2]))
	}
	if s, ok := f.DecodeCode(codes[0]); !ok || s != "中" {
		t.Fatalf("decode %q %v", s, ok)
	}
	if s, ok := f.DecodeCode(codes[1]); !ok || s != "fi" {
		t.Fatalf("decode %q %v", s, ok)
	}
	if s, ok := f.DecodeCode(codes[2]); !ok || s != "一" {
		t.Fatalf("decode %q %v", s, ok)
	}
	if f.Info.Kind != "CIDFontType2" || f.Info.BaseFont != "SimSun" {
		t.Fatalf("info %#v", f.Info)
	}
}

func TestType0PredefinedUniGB(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(5, "<< /Type /Font /Subtype /Type0 /BaseFont /SimSun /Encoding /UniGB-UCS2-H /DescendantFonts [6 0 R] >>"),
		testkit.Raw(6, "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /SimSun /DW 1000 /W [4559 [900]] >>"),
	}
	file, dc := type0FileWith(t, objs)
	defer file.Close()
	o, _ := file.ResolveRef(context.Background(), syntax.Ref{Num: 5})
	f, err := Load(context.Background(), file, o.(syntax.Dict), dc)
	if err != nil {
		t.Fatal(err)
	}
	if f.predefined == nil || f.CodeBytes() != 2 {
		t.Fatalf("predefined=%v bytes=%d", f.predefined != nil, f.CodeBytes())
	}
	codes, used := f.Split([]byte{0x4E, 0x2D, 0x00})
	if used != 2 || len(codes) != 1 {
		t.Fatalf("used=%d codes=%d", used, len(codes))
	}
	if codes[0].CID != 4559 || codes[0].Value != 0x4E2D {
		t.Fatalf("code %#v", codes[0])
	}
	if s, ok := f.DecodeCode(codes[0]); !ok || s != "中" {
		t.Fatalf("decode %q %v", s, ok)
	}
	if w := f.WidthCode(codes[0]); w != 900 {
		t.Fatalf("width %v", w)
	}
	if hasDiag(dc, "FNT-CMAP-DEFERRED") || hasDiag(dc, "FNT-NO-TOUNICODE") {
		t.Fatalf("unexpected fallback diagnostics: %v", diagCodes(dc))
	}
}

func TestType0PredefinedGBK(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(5, "<< /Type /Font /Subtype /Type0 /BaseFont /SimSun /Encoding /GBK-EUC-H /DescendantFonts [6 0 R] >>"),
		testkit.Raw(6, "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /SimSun /DW 1000 /W [4559 [900] 3795 [800]] >>"),
	}
	file, dc := type0FileWith(t, objs)
	defer file.Close()
	o, _ := file.ResolveRef(context.Background(), syntax.Ref{Num: 5})
	f, err := Load(context.Background(), file, o.(syntax.Dict), dc)
	if err != nil {
		t.Fatal(err)
	}
	codes, used := f.Split([]byte{0xD6, 0xD0, 0xCE, 0xC4})
	if used != 4 || len(codes) != 2 {
		t.Fatalf("used=%d codes=%d", used, len(codes))
	}
	if codes[0].CID != 4559 || codes[1].CID != 3795 {
		t.Fatalf("cids %d %d", codes[0].CID, codes[1].CID)
	}
	if s, ok := f.DecodeCode(codes[0]); !ok || s != "中" {
		t.Fatalf("decode0 %q %v", s, ok)
	}
	if s, ok := f.DecodeCode(codes[1]); !ok || s != "文" {
		t.Fatalf("decode1 %q %v", s, ok)
	}
	if f.WidthCode(codes[0]) != 900 || f.WidthCode(codes[1]) != 800 {
		t.Fatalf("widths %v %v", f.WidthCode(codes[0]), f.WidthCode(codes[1]))
	}
}

func TestType0PredefinedUTF16(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(5, "<< /Type /Font /Subtype /Type0 /BaseFont /SimSun /Encoding /UniGB-UTF16-H /DescendantFonts [6 0 R] >>"),
		testkit.Raw(6, "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /SimSun /DW 1000 >>"),
	}
	file, dc := type0FileWith(t, objs)
	defer file.Close()
	o, _ := file.ResolveRef(context.Background(), syntax.Ref{Num: 5})
	f, err := Load(context.Background(), file, o.(syntax.Dict), dc)
	if err != nil {
		t.Fatal(err)
	}
	codes, used := f.Split([]byte{0xD8, 0x3D, 0xDE, 0x00})
	if used != 4 || len(codes) != 1 || codes[0].Bytes != 4 {
		t.Fatalf("used=%d codes=%#v", used, codes)
	}
	if s, ok := f.DecodeCode(codes[0]); !ok || s != "😀" {
		t.Fatalf("decode %q %v", s, ok)
	}
}

func TestType0PredefinedVertical(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(5, "<< /Type /Font /Subtype /Type0 /BaseFont /SimSun /Encoding /GBK-EUC-V /DescendantFonts [6 0 R] >>"),
		testkit.Raw(6, "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /SimSun /DW 1000 >>"),
	}
	file, dc := type0FileWith(t, objs)
	defer file.Close()
	o, _ := file.ResolveRef(context.Background(), syntax.Ref{Num: 5})
	f, err := Load(context.Background(), file, o.(syntax.Dict), dc)
	if err != nil {
		t.Fatal(err)
	}
	if !f.Vertical() {
		t.Fatal("vertical flag not set")
	}
	codes := f.Codes([]byte{0xA1, 0xA2})
	if len(codes) != 1 || codes[0].CID != 575 {
		t.Fatalf("codes %#v", codes)
	}
	if f.VerticalWidth(codes[0]) != 880 {
		t.Fatalf("vertical width %v", f.VerticalWidth(codes[0]))
	}
	if hasDiag(dc, "FNT-VERTICAL-DEFERRED") || hasDiag(dc, "FNT-W2-DEFERRED") {
		t.Fatalf("unexpected vertical diagnostics: %v", diagCodes(dc))
	}
}

func TestType0VerticalWidths(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(5, "<< /Type /Font /Subtype /Type0 /BaseFont /SimSun /Encoding /Identity-V /DescendantFonts [6 0 R] >>"),
		testkit.Raw(6, "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /SimSun /DW 1000 /DW2 [900 -1000] /W2 [1 [700 250 900] 2 4 800] >>"),
	}
	file, dc := type0FileWith(t, objs)
	defer file.Close()
	o, _ := file.ResolveRef(context.Background(), syntax.Ref{Num: 5})
	f, err := Load(context.Background(), file, o.(syntax.Dict), dc)
	if err != nil {
		t.Fatal(err)
	}
	if w := f.VerticalWidth(Code{CID: 1}); w != 700 {
		t.Fatalf("cid1 %v", w)
	}
	for _, cid := range []uint32{2, 3, 4} {
		if w := f.VerticalWidth(Code{CID: cid}); w != 800 {
			t.Fatalf("cid%d %v", cid, w)
		}
	}
	if w := f.VerticalWidth(Code{CID: 9}); w != 900 {
		t.Fatalf("default %v", w)
	}
}

func TestType0UnknownCMapDeferred(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(5, "<< /Type /Font /Subtype /Type0 /BaseFont /SimSun /Encoding /Unknown-CMap-H /DescendantFonts [6 0 R] >>"),
		testkit.Raw(6, "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /SimSun /DW 1000 >>"),
	}
	file, dc := type0FileWith(t, objs)
	defer file.Close()
	o, _ := file.ResolveRef(context.Background(), syntax.Ref{Num: 5})
	f, err := Load(context.Background(), file, o.(syntax.Dict), dc)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := f.DecodeCode(Code{Value: 1, Bytes: 2, CID: 1}); ok {
		t.Fatal("should not claim Unicode")
	}
	for _, want := range []string{"FNT-CMAP-DEFERRED", "FNT-NO-TOUNICODE"} {
		if !hasDiag(dc, want) {
			t.Fatalf("missing %s in %v", want, diagCodes(dc))
		}
	}
}

func type0FileWith(t *testing.T, objs []testkit.Obj) (*syntax.File, *diag.Collector) {
	t.Helper()
	b := budget.DefaultBudget
	dc := diag.New(100)
	f, err := syntax.Open(context.Background(), testkit.Build(objs, "/Root 5 0 R"), syntax.Options{Tracker: budget.NewTracker(&b), Diag: dc})
	if err != nil {
		t.Fatal(err)
	}
	return f, dc
}

func hasDiag(dc *diag.Collector, code string) bool {
	for _, d := range dc.All() {
		if d.Code == code {
			return true
		}
	}
	return false
}

func diagCodes(dc *diag.Collector) []string {
	var out []string
	for _, d := range dc.All() {
		out = append(out, d.Code)
	}
	return out
}

func TestSimpleFontToUnicode(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(5, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /ToUnicode 6 0 R >>"),
		testkit.Stream(6, "", []byte(sampleCMap)),
	}
	file, dc := type0FileWith(t, objs)
	defer file.Close()
	o, _ := file.ResolveRef(context.Background(), syntax.Ref{Num: 5})
	f, err := Load(context.Background(), file, o.(syntax.Dict), dc)
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := f.Decode(1); !ok || s != "中" {
		t.Fatalf("ToUnicode decode %q %v", s, ok)
	}
	if s, ok := f.Decode('A'); !ok || s != "A" {
		t.Fatalf("encoding fallback %q %v", s, ok)
	}
}

func TestType3FontMetrics(t *testing.T) {
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
		testkit.Raw(5, "<< /Type /Font /Subtype /Type3 /FontBBox [0 0 500 500] /FontMatrix [0.002 0 0 0.002 0 0] /CharProcs << /gA 6 0 R >> /Encoding << /Differences [65 /gA] >> /FirstChar 65 /LastChar 65 /Widths [500] /ToUnicode 7 0 R >>"),
		testkit.Stream(6, "", []byte("500 0 0 0 500 500 d1 0 0 100 100 re f")),
		testkit.Stream(7, "", []byte(cmap)),
	}
	file, dc := type0FileWith(t, objs)
	defer file.Close()
	o, _ := file.ResolveRef(context.Background(), syntax.Ref{Num: 5})
	f, err := Load(context.Background(), file, o.(syntax.Dict), dc)
	if err != nil {
		t.Fatal(err)
	}
	if !f.Type3() || f.GlyphName(65) != "gA" {
		t.Fatalf("type3=%v name=%q", f.Type3(), f.GlyphName(65))
	}
	if w := f.Width(65); w != 1000 {
		t.Fatalf("width %v", w)
	}
	if s, ok := f.Decode(65); !ok || s != "中" {
		t.Fatalf("decode %q %v", s, ok)
	}
	glyph, ok, err := f.Type3Proc(context.Background(), file, 65)
	if err != nil || !ok || len(glyph.Data) == 0 {
		t.Fatalf("proc ok=%v err=%v data=%d", ok, err, len(glyph.Data))
	}
	if glyph.Matrix != [6]float64{0.002, 0, 0, 0.002, 0, 0} {
		t.Fatalf("matrix %#v", glyph.Matrix)
	}
}
