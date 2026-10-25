package render

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/F31/go-pdf/displaylist"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
)

func writeSystemFont(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSystemFonts(t *testing.T) {
	dir := t.TempDir()
	writeSystemFont(t, dir, "Go-Regular.ttf", goregular.TTF)
	writeSystemFont(t, dir, "Go-Bold.ttf", gobold.TTF)
	writeSystemFont(t, dir, "GoMono-Regular.ttf", gomono.TTF)
	fonts, err := SystemFonts(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := fonts.GlyphOutline(&displaylist.FontInfo{Family: "Go", BaseFont: "Go"}, displaylist.Glyph{Text: "H"}); err != nil || !ok {
		t.Fatalf("Go outline = %v, %v", ok, err)
	}
	if _, ok, err := fonts.GlyphOutline(&displaylist.FontInfo{Family: "Go", Bold: true}, displaylist.Glyph{Text: "H"}); err != nil || !ok {
		t.Fatalf("Go Bold outline = %v, %v", ok, err)
	}
	if _, ok, err := fonts.GlyphOutline(&displaylist.FontInfo{Family: "Go Mono", Mono: true}, displaylist.Glyph{Text: "M"}); err != nil || !ok {
		t.Fatalf("Go Mono outline = %v, %v", ok, err)
	}
	if _, ok, err := fonts.GlyphOutline(&displaylist.FontInfo{BaseFont: "Helvetica"}, displaylist.Glyph{Text: "H"}); err != nil || !ok {
		t.Fatalf("generic fallback = %v, %v", ok, err)
	}
	if _, ok, err := fonts.GlyphOutline(&displaylist.FontInfo{Family: "Go"}, displaylist.Glyph{Text: "😀"}); err != nil || ok {
		t.Fatalf("missing glyph = %v, %v", ok, err)
	}
}

func TestSystemFontsEmpty(t *testing.T) {
	if _, err := SystemFonts(t.TempDir()); err == nil {
		t.Fatal("empty font directory accepted")
	}
}

func buildTTC(fonts ...[]byte) []byte {
	header := 12 + 4*len(fonts)
	offsets := make([]uint32, len(fonts))
	total := header
	for i, f := range fonts {
		offsets[i] = uint32(total)
		total += len(f)
	}
	out := make([]byte, total)
	copy(out[:4], "ttcf")
	binary.BigEndian.PutUint32(out[4:8], 0x00010000)
	binary.BigEndian.PutUint32(out[8:12], uint32(len(fonts)))
	for i, off := range offsets {
		binary.BigEndian.PutUint32(out[12+4*i:], off)
	}
	for i, f := range fonts {
		base := int(offsets[i])
		copy(out[base:], f)
		count := int(binary.BigEndian.Uint16(out[base+4 : base+6]))
		for j := 0; j < count; j++ {
			rec := base + 12 + 16*j
			old := binary.BigEndian.Uint32(out[rec+8 : rec+12])
			binary.BigEndian.PutUint32(out[rec+8:rec+12], old+uint32(base))
		}
	}
	return out
}

func TestSystemFontsTTC(t *testing.T) {
	dir := t.TempDir()
	writeSystemFont(t, dir, "Go.ttc", buildTTC(goregular.TTF, gomono.TTF))
	fonts, err := SystemFonts(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := fonts.GlyphOutline(&displaylist.FontInfo{Family: "Go"}, displaylist.Glyph{Text: "H"}); err != nil || !ok {
		t.Fatalf("TTC Go = %v, %v", ok, err)
	}
	if _, ok, err := fonts.GlyphOutline(&displaylist.FontInfo{Family: "Go Mono", Mono: true}, displaylist.Glyph{Text: "M"}); err != nil || !ok {
		t.Fatalf("TTC Go Mono = %v, %v", ok, err)
	}
}

func TestCJKDetection(t *testing.T) {
	for _, r := range []rune{'中', '日', '한', 'あ', 'ア'} {
		if !isCJKRune(r) {
			t.Fatalf("%q not detected as CJK", r)
		}
	}
	if isCJKRune('A') {
		t.Fatal("ASCII detected as CJK")
	}
	for _, name := range []string{"Noto Sans CJK SC", "WenQuanYi Zen Hei", "SimSun", "Microsoft YaHei", "宋体"} {
		if !isCJKFamily(name) {
			t.Fatalf("%q not detected as CJK family", name)
		}
	}
}

func TestCJKLocale(t *testing.T) {
	cases := map[string]string{
		"Noto Sans CJK SC":   "zh-Hans",
		"Noto Sans CJK TC":   "zh-Hant",
		"Noto Sans CJK HK":   "zh-Hant",
		"Noto Sans CJK JP":   "ja",
		"Noto Sans CJK KR":   "ko",
		"Source Han Sans SC": "zh-Hans",
		"SimSun":             "zh-Hans",
		"MingLiU":            "zh-Hant",
		"MS Mincho":          "ja",
		"Malgun Gothic":      "ko",
		"DejaVu Sans":        "",
		"NotoSansCJKsc":      "zh-Hans",
		"NotoSansCJKjp":      "ja",
	}
	for name, want := range cases {
		if got := cjkLocaleFromName(name); got != want {
			t.Fatalf("locale(%q) = %q, want %q", name, got, want)
		}
	}
	if cjkLocaleFromRune('あ') != "ja" || cjkLocaleFromRune('한') != "ko" || cjkLocaleFromRune('中') != "" {
		t.Fatal("rune locale mismatch")
	}
}

func TestCJKCandidateLocaleOrder(t *testing.T) {
	ja := &systemFontEntry{family: "cjks", cjk: true, locale: "ja"}
	sc := &systemFontEntry{family: "cjkt", cjk: true, locale: "zh-Hans"}
	p := &systemFontProvider{all: []*systemFontEntry{ja, sc}}
	if got := p.candidates(&displaylist.FontInfo{Family: "SimSun"}, '中'); len(got) == 0 || got[0] != sc {
		t.Fatalf("Hans order = %#v", got)
	}
	if got := p.candidates(&displaylist.FontInfo{}, 'あ'); len(got) == 0 || got[0] != ja {
		t.Fatalf("Japanese order = %#v", got)
	}
}
