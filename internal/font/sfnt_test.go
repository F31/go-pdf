package font

import (
	"context"
	"encoding/binary"
	"os"
	"testing"

	"golang.org/x/image/font/sfnt"

	"github.com/F31/go-pdf/internal/syntax"
	"github.com/F31/go-pdf/internal/testkit"
)

func testFontData(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../../testdata/fonts/gopdf-test-sans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestSimpleFontMetrics(t *testing.T) {
	fontData := testFontData(t)
	objs := []testkit.Obj{
		testkit.Raw(5, "<< /Type /Font /Subtype /TrueType /BaseFont /GoPDFTestSans /Encoding /WinAnsiEncoding /FontDescriptor 6 0 R >>"),
		testkit.Raw(6, "<< /Type /FontDescriptor /FontName /GoPDFTestSans /Flags 32 /FontBBox [0 0 1000 1000] /ItalicAngle 0 /Ascent 905 /Descent -212 /CapHeight 700 /StemV 80 /FontFile2 7 0 R >>"),
		testkit.Stream(7, "", fontData),
	}
	file, dc := type0FileWith(t, objs)
	defer file.Close()
	o, _ := file.ResolveRef(context.Background(), syntax.Ref{Num: 5})
	f, err := Load(context.Background(), file, o.(syntax.Dict), dc)
	if err != nil {
		t.Fatal(err)
	}
	m := f.Info.Metrics
	if !f.Info.Embedded || m.Substituted {
		t.Fatalf("embedded=%v substituted=%v", f.Info.Embedded, m.Substituted)
	}
	if m.UnitsPerEm != 2048 || m.Ascent != 1854 || m.Descent != 434 || m.CapHeight != 1409 || m.XHeight != 1082 {
		t.Fatalf("metrics %#v", m)
	}
	want := map[uint32]int{72: 722, 101: 556, 108: 222, 111: 556}
	for code, adv := range want {
		if got := m.Advances[code]; got != adv {
			t.Fatalf("advance %d = %d want %d (all %#v)", code, got, adv, m.Advances)
		}
	}
	if m.AvgWidth != 1187 {
		t.Fatalf("avg width %d", m.AvgWidth)
	}
	if f.Info.Ascent != 905 || f.Info.Descent != -212 {
		t.Fatalf("descriptor ascent %v descent %v", f.Info.Ascent, f.Info.Descent)
	}
	if hasDiag(dc, "FNT-SFNT-PARSE") {
		t.Fatalf("unexpected diagnostics: %v", diagCodes(dc))
	}
}

func TestCIDFontMetrics(t *testing.T) {
	fontData := testFontData(t)
	cmap := `
begincmap
1 begincodespacerange
<0000> <FFFF>
endcodespacerange
4 beginbfchar
<0001> <0048>
<0002> <0065>
<0003> <006C>
<0004> <006F>
endbfchar
endcmap
`
	cidToGID := []byte{0, 0, 0, 4, 0, 1, 0, 2, 0, 3}
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
	m := f.Info.Metrics
	if m.Substituted || m.UnitsPerEm != 2048 {
		t.Fatalf("metrics %#v", m)
	}
	want := map[uint32]int{1: 556, 2: 722, 3: 556, 4: 222}
	for cid, adv := range want {
		if got := m.Advances[cid]; got != adv {
			t.Fatalf("advance cid %d = %d want %d (all %#v)", cid, got, adv, m.Advances)
		}
	}
}

func TestInvalidFontProgram(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(5, "<< /Type /Font /Subtype /TrueType /BaseFont /Broken /Encoding /WinAnsiEncoding /FontDescriptor 6 0 R >>"),
		testkit.Raw(6, "<< /Type /FontDescriptor /FontName /Broken /Flags 32 /FontBBox [0 0 1000 1000] /ItalicAngle 0 /Ascent 0 /Descent 0 /CapHeight 0 /StemV 80 /FontFile2 7 0 R >>"),
		testkit.Stream(7, "", []byte("not a font program")),
	}
	file, dc := type0FileWith(t, objs)
	defer file.Close()
	o, _ := file.ResolveRef(context.Background(), syntax.Ref{Num: 5})
	f, err := Load(context.Background(), file, o.(syntax.Dict), dc)
	if err != nil {
		t.Fatal(err)
	}
	if !f.Info.Metrics.Substituted {
		t.Fatal("metrics should be substituted")
	}
	if !hasDiag(dc, "FNT-SFNT-PARSE") {
		t.Fatalf("missing parse diagnostic: %v", diagCodes(dc))
	}
}

func TestAlignSFNTTables(t *testing.T) {
	data := testFontData(t)
	numTables := int(binary.BigEndian.Uint16(data[4:6]))
	header := 12 + 16*numTables
	shifted := make([]byte, len(data)+2)
	copy(shifted[:header], data[:header])
	copy(shifted[header+2:], data[header:])
	for i := 0; i < numTables; i++ {
		off := binary.BigEndian.Uint32(shifted[12+16*i+8:])
		binary.BigEndian.PutUint32(shifted[12+16*i+8:], off+2)
	}
	repaired, changed := normalizeSFNTTables(shifted)
	if !changed {
		t.Fatal("expected misaligned tables to be repaired")
	}
	if _, err := sfnt.Parse(repaired); err != nil {
		t.Fatalf("repaired font: %v", err)
	}
}

func TestRepairMetricTables(t *testing.T) {
	data := append([]byte(nil), testFontData(t)...)
	records, ok := readSFNTRecords(data)
	if !ok {
		t.Fatal("invalid test font")
	}
	found := false
	for _, r := range records {
		if r.tag == 0x636d6170 {
			for i := uint32(0); i < 4 && i < r.length; i++ {
				data[r.offset+i] = 0
			}
			found = true
		}
	}
	if !found {
		t.Fatal("test font has no cmap")
	}
	if _, err := sfnt.Parse(data); err == nil {
		t.Fatal("expected corrupted cmap to fail")
	}
	repaired, changed := repairMetricTables(data)
	if !changed {
		t.Fatal("expected table repair")
	}
	face, err := sfnt.Parse(repaired)
	if err != nil {
		t.Fatalf("repaired font: %v", err)
	}
	if int(face.UnitsPerEm()) != 2048 {
		t.Fatalf("units per em %d", face.UnitsPerEm())
	}
}

func TestRepairMissingTables(t *testing.T) {
	data := testFontData(t)
	records, ok := readSFNTRecords(data)
	if !ok {
		t.Fatal("invalid test font")
	}
	without := records[:0]
	for _, r := range records {
		if r.tag != 0x636d6170 {
			without = append(without, r)
		}
	}
	missing := rebuildSFNT(without, data, nil)
	if _, err := sfnt.Parse(missing); err == nil {
		t.Fatal("expected missing cmap to fail")
	}
	repaired, changed := repairMetricTables(missing)
	if !changed {
		t.Fatal("expected table repair")
	}
	if _, err := sfnt.Parse(repaired); err != nil {
		t.Fatalf("repaired font: %v", err)
	}
}

func FuzzSFNTRepair(f *testing.F) {
	if data, err := os.ReadFile("../../testdata/fonts/gopdf-test-sans.ttf"); err == nil {
		f.Add(data)
	}
	f.Add([]byte{})
	f.Add([]byte{0, 1, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0})
	f.Fuzz(func(t *testing.T, data []byte) {
		normalized, _ := normalizeSFNTTables(data)
		if repaired, changed := repairMetricTables(normalized); changed {
			_, _ = sfnt.Parse(repaired)
		}
	})
}
