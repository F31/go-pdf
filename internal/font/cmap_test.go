package font

import "testing"

const sampleCMap = `
/CIDInit /ProcSet findresource begin
12 dict begin
begincmap
/CMapName /Adobe-Identity-UCS def
1 begincodespacerange
<0000> <FFFF>
endcodespacerange
2 beginbfchar
<0001> <4E2D>
<0002> <00660069>
endbfchar
2 beginbfrange
<0010> <0012> <4E00>
<0020> <0021> [<4E2D> <6587>]
endbfrange
endcmap
end
end
`

func TestParseToUnicode(t *testing.T) {
	cm, err := ParseToUnicode([]byte(sampleCMap))
	if err != nil {
		t.Fatal(err)
	}
	if cm.CodeLen != 2 {
		t.Fatalf("code len %d", cm.CodeLen)
	}
	if got := cm.Map[1]; got != "中" {
		t.Fatalf("1 = %q", got)
	}
	if got := cm.Map[2]; got != "fi" {
		t.Fatalf("2 = %q", got)
	}
	if got := cm.Map[0x10]; got != "一" {
		t.Fatalf("10 = %q", got)
	}
	if got := cm.Map[0x12]; got != "丂" {
		t.Fatalf("12 = %q", got)
	}
	if got := cm.Map[0x20]; got != "中" {
		t.Fatalf("20 = %q", got)
	}
	if got := cm.Map[0x21]; got != "文" {
		t.Fatalf("21 = %q", got)
	}
}

func TestParseToUnicodeTruncated(t *testing.T) {
	cm, _ := ParseToUnicode([]byte("begincmap 1 beginbfchar <0001>"))
	if cm == nil {
		t.Fatal("nil cmap")
	}
	if _, ok := cm.Map[1]; ok {
		t.Fatal("unexpected mapping")
	}
}

func FuzzParseToUnicode(f *testing.F) {
	f.Add([]byte(sampleCMap))
	f.Add([]byte("begincmap"))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = ParseToUnicode(data)
	})
}
