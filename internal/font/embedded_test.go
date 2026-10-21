package font

import (
	"context"
	"testing"

	"github.com/F31/go-pdf/internal/syntax"
	"github.com/F31/go-pdf/internal/testkit"
)

func TestParseEmbeddedCMap(t *testing.T) {
	data := []byte(`
begincmap
/WMode 0 def
1 begincodespacerange
<0000> <FFFF>
endcodespacerange
1 begincidrange
<0001> <0003> 500
endcidrange
1 beginnotdefrange
<0000> <0000> 1
endnotdefrange
endcmap
`)
	cm, err := parseEmbeddedCMap(data)
	if err != nil {
		t.Fatal(err)
	}
	if cm.maxBytes != 2 || len(cm.spaces) != 1 {
		t.Fatalf("max=%d spaces=%#v", cm.maxBytes, cm.spaces)
	}
	for code, want := range map[uint32]uint32{0: 1, 1: 500, 2: 501, 3: 502} {
		if got := cm.cidFor(2, code); got != want {
			t.Fatalf("code %d cid %d want %d", code, got, want)
		}
	}
	if cm.hasUnicode() {
		t.Fatal("unexpected Unicode fallback")
	}
}

func TestParseEmbeddedCMapUseGBK(t *testing.T) {
	data := []byte(`
/GBK-EUC-H usecmap
1 begincidrange
<D6D0> <D6D0> 999
endcidrange
`)
	cm, err := parseEmbeddedCMap(data)
	if err != nil {
		t.Fatal(err)
	}
	codes, used := cm.split([]byte{0xD6, 0xD0})
	if used != 2 || len(codes) != 1 || codes[0].CID != 999 {
		t.Fatalf("used=%d codes=%#v", used, codes)
	}
	if s, ok := cm.decode(codes[0]); !ok || s != "中" {
		t.Fatalf("decode %q %v", s, ok)
	}
}

func TestParseEmbeddedCMapUseIdentity(t *testing.T) {
	data := []byte(`
/Identity-H usecmap
1 begincidrange
<0001> <0001> 42
endcidrange
`)
	cm, err := parseEmbeddedCMap(data)
	if err != nil {
		t.Fatal(err)
	}
	codes, used := cm.split([]byte{0x00, 0x01, 0x00, 0x02})
	if used != 4 || len(codes) != 2 {
		t.Fatalf("used=%d codes=%#v", used, codes)
	}
	if codes[0].CID != 42 || codes[1].CID != 2 {
		t.Fatalf("cids %#v", codes)
	}
}

func TestEmbeddedCMapFont(t *testing.T) {
	cmap := `
begincmap
1 begincodespacerange
<0000> <FFFF>
endcodespacerange
1 begincidrange
<0001> <0002> 500
endcidrange
endcmap
`
	objs := []testkit.Obj{
		testkit.Raw(5, "<< /Type /Font /Subtype /Type0 /BaseFont /SimSun /Encoding 7 0 R /DescendantFonts [6 0 R] /ToUnicode 8 0 R >>"),
		testkit.Raw(6, "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /SimSun /DW 1000 /W [500 [700 800]] >>"),
		testkit.Stream(7, "", []byte(cmap)),
		testkit.Stream(8, "", []byte(sampleCMap)),
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
	codes := f.Codes([]byte{0, 1, 0, 2})
	if len(codes) != 2 || codes[0].CID != 500 || codes[1].CID != 501 {
		t.Fatalf("codes %#v", codes)
	}
	if f.WidthCode(codes[0]) != 700 || f.WidthCode(codes[1]) != 800 {
		t.Fatalf("widths %v %v", f.WidthCode(codes[0]), f.WidthCode(codes[1]))
	}
	if s, ok := f.DecodeCode(codes[0]); !ok || s != "中" {
		t.Fatalf("decode %q %v", s, ok)
	}
	if hasDiag(dc, "FNT-CMAP-EMBEDDED") || hasDiag(dc, "FNT-CMAP-DEFERRED") {
		t.Fatalf("unexpected diagnostics: %v", diagCodes(dc))
	}
}

func TestEmbeddedCMapInvalid(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(5, "<< /Type /Font /Subtype /Type0 /BaseFont /SimSun /Encoding 7 0 R /DescendantFonts [6 0 R] >>"),
		testkit.Raw(6, "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /SimSun /DW 1000 >>"),
		testkit.Stream(7, "", []byte("begincmap 1 begincidrange <0001>")),
	}
	file, dc := type0FileWith(t, objs)
	defer file.Close()
	o, _ := file.ResolveRef(context.Background(), syntax.Ref{Num: 5})
	f, err := Load(context.Background(), file, o.(syntax.Dict), dc)
	if err != nil {
		t.Fatal(err)
	}
	if f.CodeBytes() != 2 {
		t.Fatalf("fallback bytes %d", f.CodeBytes())
	}
	if !hasDiag(dc, "FNT-CMAP-EMBEDDED") {
		t.Fatalf("missing embedded diagnostic: %v", diagCodes(dc))
	}
}

func FuzzParseEmbeddedCMap(f *testing.F) {
	f.Add([]byte("begincmap 1 begincodespacerange <00> <FF> endcodespacerange 1 begincidrange <01> <02> 10 endcidrange endcmap"))
	f.Add([]byte("/Identity-H usecmap 1 begincidrange <0001> <0001> 42 endcidrange"))
	f.Add([]byte("begincmap"))
	f.Add([]byte("/GBK-EUC-H usecmap 1 begincidchar <D6D0> 999 endcidchar"))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = parseEmbeddedCMap(data)
	})
}
