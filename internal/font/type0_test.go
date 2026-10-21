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

func TestType0NoToUnicode(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(5, "<< /Type /Font /Subtype /Type0 /BaseFont /SimSun /Encoding /UniGB-UCS2-H /DescendantFonts [6 0 R] >>"),
		testkit.Raw(6, "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /SimSun /DW 1000 >>"),
	}
	b := budget.DefaultBudget
	dc := diag.New(100)
	file, err := syntax.Open(context.Background(), testkit.Build(objs, "/Root 5 0 R"), syntax.Options{Tracker: budget.NewTracker(&b), Diag: dc})
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	o, _ := file.ResolveRef(context.Background(), syntax.Ref{Num: 5})
	f, err := Load(context.Background(), file, o.(syntax.Dict), dc)
	if err != nil {
		t.Fatal(err)
	}
	if f.CodeBytes() != 2 {
		t.Fatalf("bytes %d", f.CodeBytes())
	}
	if _, ok := f.DecodeCode(Code{Value: 1, Bytes: 2, CID: 1}); ok {
		t.Fatal("should not claim Unicode")
	}
	codes := map[string]bool{}
	for _, d := range dc.All() {
		codes[d.Code] = true
	}
	for _, want := range []string{"FNT-CMAP-DEFERRED", "FNT-NO-TOUNICODE"} {
		if !codes[want] {
			t.Fatalf("missing %s in %v", want, codes)
		}
	}
}
