package text

import (
	"math"
	"testing"

	"github.com/F31/go-pdf/displaylist"
)

func testRun(text string, x, y float64) *displaylist.TextRun {
	return testRunStruct(text, x, y, -1)
}

func testRunStruct(text string, x, y float64, structRef int) *displaylist.TextRun {
	return testRunStep(text, x, y, 5, structRef)
}

func testRunStep(text string, x, y, step float64, structRef int) *displaylist.TextRun {
	glyphs := make([]displaylist.Glyph, 0, len([]rune(text)))
	for i, r := range []rune(text) {
		glyphs = append(glyphs, displaylist.Glyph{
			Text: string(r), UnicodeOK: true,
			Origin:  displaylist.Point{X: x + float64(i)*step, Y: y},
			Advance: 5,
		})
	}
	return &displaylist.TextRun{Common: displaylist.Common{StructRef: structRef}, Size: 10, Glyphs: glyphs}
}

func TestMultiColumnReadingOrder(t *testing.T) {
	dl := &displaylist.Page{
		Width: 595, Height: 842,
		Items: []displaylist.Item{
			testRun("Left1", 10, 50),
			testRun("Right1", 300, 50),
			testRun("Left2", 10, 70),
			testRun("Right2", 300, 70),
		},
	}
	p := Extract(dl, Options{})
	if p.Columns != 2 {
		t.Fatalf("columns = %d, text = %q", p.Columns, p.Text)
	}
	if p.Text != "Left1\nLeft2\nRight1\nRight2" {
		t.Fatalf("text = %q", p.Text)
	}
	if p.Lines[0].Column != 0 || p.Lines[2].Column != 1 {
		t.Fatalf("line columns = %#v", p.Lines)
	}
}

func TestFullWidthHeaderBeforeColumns(t *testing.T) {
	dl := &displaylist.Page{
		Width: 595, Height: 842,
		Items: []displaylist.Item{
			testRun("Header spanning both columns across the page width", 10, 20),
			testRun("Left", 10, 50),
			testRun("Right", 300, 50),
		},
	}
	p := Extract(dl, Options{})
	if p.Text != "Header spanning both columns across the page width\nLeft\nRight" {
		t.Fatalf("text = %q columns = %d", p.Text, p.Columns)
	}
}

func TestSingleColumnOrder(t *testing.T) {
	dl := &displaylist.Page{
		Width: 595, Height: 842,
		Items: []displaylist.Item{
			testRun("second", 10, 70),
			testRun("first", 10, 50),
		},
	}
	p := Extract(dl, Options{})
	if p.Text != "first\nsecond" || p.Columns != 1 {
		t.Fatalf("text = %q columns = %d", p.Text, p.Columns)
	}
}

func TestTaggedStructureOrder(t *testing.T) {
	dl := &displaylist.Page{
		Width: 595, Height: 842,
		Items: []displaylist.Item{
			testRunStruct("Second", 10, 50, 2),
			testRunStruct("First", 10, 70, 1),
		},
	}
	p := Extract(dl, Options{
		StructOrder: []int{1, 2},
		StructInfo: map[int]StructInfo{
			1: {Type: "H1", Title: "First", Alt: "Alt first", Lang: "en"},
			2: {Type: "P", Lang: "he"},
		},
	})
	if p.Text != "First\nSecond" {
		t.Fatalf("text = %q", p.Text)
	}
	if p.Blocks[0].Type != "H1" || p.Blocks[0].Alt != "Alt first" || p.Blocks[1].Lang != "he" {
		t.Fatalf("blocks = %#v", p.Blocks)
	}
	if len(p.Blocks) != 2 || p.Lines[0].Block != 0 || p.Lines[1].Block != 1 {
		t.Fatalf("blocks = %#v lines = %#v", p.Blocks, p.Lines)
	}
}

func TestBlocksGroupLines(t *testing.T) {
	dl := &displaylist.Page{
		Width: 595, Height: 842,
		Items: []displaylist.Item{
			testRun("First line", 10, 50),
			testRun("Second line", 10, 70),
		},
	}
	p := Extract(dl, Options{})
	if len(p.Blocks) != 1 || p.Blocks[0].Text != "First line\nSecond line" {
		t.Fatalf("blocks = %#v", p.Blocks)
	}
	if len(p.Lines) != 2 || p.Lines[0].Block != 0 || p.Lines[1].Block != 0 {
		t.Fatalf("lines = %#v", p.Lines)
	}
}

func verticalRun(text string, x, y, step float64) *displaylist.TextRun {
	glyphs := make([]displaylist.Glyph, 0, len([]rune(text)))
	for i, r := range []rune(text) {
		glyphs = append(glyphs, displaylist.Glyph{
			Text: string(r), UnicodeOK: true,
			Origin: displaylist.Point{X: x, Y: y + float64(i)*step}, Advance: 5,
		})
	}
	return &displaylist.TextRun{Size: 10, Vertical: true, Glyphs: glyphs}
}

func TestVerticalColumnOrder(t *testing.T) {
	dl := &displaylist.Page{
		Width: 300, Height: 300,
		Items: []displaylist.Item{
			verticalRun("AB", 200, 50, 10),
			verticalRun("CD", 100, 50, 10),
		},
	}
	p := Extract(dl, Options{})
	if !p.Vertical || p.Text != "AB\nCD" || p.Columns != 2 {
		t.Fatalf("text=%q vertical=%v columns=%d", p.Text, p.Vertical, p.Columns)
	}
	if p.Lines[0].Column != 0 || p.Lines[1].Column != 1 {
		t.Fatalf("columns = %d,%d", p.Lines[0].Column, p.Lines[1].Column)
	}
	if math.Abs(p.Chars[0].Rotation-90) > 0.01 || math.Abs(p.Lines[0].Rotation-90) > 0.01 {
		t.Fatalf("rotation char=%v line=%v", p.Chars[0].Rotation, p.Lines[0].Rotation)
	}
	if p.Chars[0].Quad[1].Y <= p.Chars[0].Origin.Y {
		t.Fatalf("vertical quad = %#v", p.Chars[0].Quad)
	}
	if sel := p.Select(0, 1); len(sel.CharQuads) != 1 || sel.CharQuads[0] != p.Chars[0].Quad {
		t.Fatalf("vertical selection = %#v", sel.CharQuads)
	}
	q := Extract(dl, Options{VerticalOrder: VerticalLTR})
	if q.Text != "CD\nAB" {
		t.Fatalf("ltr text = %q", q.Text)
	}
}

func TestPureRTLReadingOrder(t *testing.T) {
	dl := &displaylist.Page{
		Width: 595, Height: 842,
		Items: []displaylist.Item{
			testRunStep("שלום", 100, 50, -5, -1),
		},
	}
	p := Extract(dl, Options{})
	if len(p.Lines) != 1 || p.Lines[0].Direction != DirectionRTL {
		t.Fatalf("lines = %#v", p.Lines)
	}
	if p.Text != "שלום" {
		t.Fatalf("rtl text = %q", p.Text)
	}
	if hits := p.Search("שלום", SearchOptions{}); len(hits) != 1 {
		t.Fatalf("rtl search hits = %d", len(hits))
	}
}

func TestMixedBidiLTRBase(t *testing.T) {
	dl := &displaylist.Page{
		Width: 595, Height: 842,
		Items: []displaylist.Item{testRun("abc םולש def", 10, 50)},
	}
	p := Extract(dl, Options{})
	if p.Text != "abc שלום def" || p.Lines[0].BaseDirection != DirectionLTR {
		t.Fatalf("text = %q base=%v", p.Text, p.Lines[0].BaseDirection)
	}
}

func TestMixedBidiRTLBase(t *testing.T) {
	dl := &displaylist.Page{
		Width: 595, Height: 842,
		Items: []displaylist.Item{testRun("םלוע abc םולש", 10, 50)},
	}
	p := Extract(dl, Options{})
	if p.Text != "שלום abc עולם" || p.Lines[0].BaseDirection != DirectionRTL {
		t.Fatalf("text = %q base=%v", p.Text, p.Lines[0].BaseDirection)
	}
}

func TestMixedDirectionIsNotReversed(t *testing.T) {
	if got := lineDirection("abc שלום"); got != DirectionMixed {
		t.Fatalf("direction = %v", got)
	}
	if got := lineDirection("hello"); got != DirectionLTR {
		t.Fatalf("direction = %v", got)
	}
}

func TestUnicodeNormalization(t *testing.T) {
	dl := &displaylist.Page{
		Width: 595, Height: 842,
		Items: []displaylist.Item{testRun("ﬁ", 10, 50)},
	}
	p := Extract(dl, Options{Normalization: NormalizationNFKC})
	if p.Text != "fi" || p.Normalization != NormalizationNFKC {
		t.Fatalf("text = %q norm = %v", p.Text, p.Normalization)
	}
	if hits := p.Search("fi", SearchOptions{}); len(hits) != 1 {
		t.Fatalf("search hits = %d", len(hits))
	}
	if hits := p.Search("ﬁ", SearchOptions{}); len(hits) != 1 {
		t.Fatalf("compat search hits = %d", len(hits))
	}
}

func TestNFCNormalization(t *testing.T) {
	dl := &displaylist.Page{
		Width: 595, Height: 842,
		Items: []displaylist.Item{testRun("e\u0301", 10, 50)},
	}
	p := Extract(dl, Options{Normalization: NormalizationNFC})
	if p.Text != "é" {
		t.Fatalf("text = %q", p.Text)
	}
}

func TestSelection(t *testing.T) {
	dl := &displaylist.Page{
		Width: 595, Height: 842,
		Items: []displaylist.Item{
			testRun("Hello world", 10, 50),
			testRun("Second line", 10, 70),
		},
	}
	p := Extract(dl, Options{})
	if p.Text != "Hello world\nSecond line" {
		t.Fatalf("text = %q", p.Text)
	}
	sel := p.Select(0, 5)
	if sel.Text != "Hello" || len(sel.Chars) != 5 || len(sel.Quads) != 1 {
		t.Fatalf("selection = %#v", sel)
	}
	sel = p.Select(6, 11)
	if sel.Text != "world" || len(sel.Quads) != 1 {
		t.Fatalf("word selection = %#v", sel)
	}
	all := p.Select(-10, 1000)
	if all.Text != p.Text || len(all.Quads) != 2 {
		t.Fatalf("all = %#v", all)
	}
	if p.Select(10, 2).Text != "llo worl" {
		t.Fatalf("swap = %q", p.Select(10, 2).Text)
	}
}

func TestSelectionNormalizedLigature(t *testing.T) {
	dl := &displaylist.Page{
		Width: 595, Height: 842,
		Items: []displaylist.Item{testRun("ﬁ", 10, 50)},
	}
	p := Extract(dl, Options{Normalization: NormalizationNFKC})
	sel := p.Select(0, 2)
	if sel.Text != "fi" || len(sel.Chars) != 1 || len(sel.Quads) != 1 {
		t.Fatalf("selection = %#v", sel)
	}
}

func TestSearchQuads(t *testing.T) {
	dl := &displaylist.Page{
		Width: 595, Height: 842,
		Items: []displaylist.Item{
			testRun("Hello", 10, 50),
			testRun("Second", 10, 70),
		},
	}
	p := Extract(dl, Options{})
	hits := p.Search("Hello Second", SearchOptions{})
	if len(hits) != 1 || len(hits[0].Quads) != 2 {
		t.Fatalf("hits = %#v", hits)
	}
	if hits[0].Quads[0].Y0 == hits[0].Quads[1].Y0 {
		t.Fatalf("quads = %#v", hits[0].Quads)
	}
}

func testRunMatrix(text string, x, y, step float64, m displaylist.Matrix) *displaylist.TextRun {
	glyphs := make([]displaylist.Glyph, 0, len([]rune(text)))
	for i, r := range []rune(text) {
		glyphs = append(glyphs, displaylist.Glyph{
			Text: string(r), UnicodeOK: true,
			Origin:  displaylist.Point{X: x + float64(i)*step, Y: y},
			Advance: 5, Matrix: m,
		})
	}
	return &displaylist.TextRun{Size: 10, Glyphs: glyphs}
}

func TestRotatedReadingOrder(t *testing.T) {
	run := &displaylist.TextRun{Size: 10, Glyphs: []displaylist.Glyph{
		{Text: "A", UnicodeOK: true, Origin: displaylist.Point{X: 50, Y: 50}, Advance: 5, Matrix: displaylist.Matrix{0, 1, -1, 0, 0, 0}},
		{Text: "B", UnicodeOK: true, Origin: displaylist.Point{X: 50, Y: 55}, Advance: 5, Matrix: displaylist.Matrix{0, 1, -1, 0, 0, 0}},
	}}
	p := Extract(&displaylist.Page{Width: 200, Height: 200, Items: []displaylist.Item{run}}, Options{})
	if len(p.Lines) != 1 || p.Lines[0].Text != "AB" || math.Abs(p.Lines[0].Rotation-90) > 0.01 {
		t.Fatalf("lines = %#v", p.Lines)
	}
	if p.Text != "AB" || len(p.Chars) != 2 || p.Chars[0].Text != "A" || p.Chars[1].Text != "B" {
		t.Fatalf("text=%q chars=%#v", p.Text, p.Chars)
	}
}

func TestRotatedTextMetadata(t *testing.T) {
	// 90 degrees counter-clockwise in page space.
	dl := &displaylist.Page{
		Width: 595, Height: 842,
		Items: []displaylist.Item{
			testRunMatrix("AB", 50, 50, 0, displaylist.Matrix{0, 1, -1, 0, 0, 0}),
		},
	}
	p := Extract(dl, Options{})
	if len(p.Chars) != 2 {
		t.Fatalf("chars = %#v", p.Chars)
	}
	if math.Abs(p.Chars[0].Rotation-90) > 0.01 || math.Abs(p.Lines[0].Rotation-90) > 0.01 {
		t.Fatalf("rotation = %v line=%v", p.Chars[0].Rotation, p.Lines[0].Rotation)
	}
	if p.Chars[0].Quad[1].Y <= p.Chars[0].Origin.Y {
		t.Fatalf("quad = %#v", p.Chars[0].Quad)
	}
	sel := p.Select(0, 1)
	if len(sel.CharQuads) != 1 || sel.CharQuads[0] != p.Chars[0].Quad {
		t.Fatalf("selection quads = %#v", sel.CharQuads)
	}
	hits := p.Search("A", SearchOptions{})
	if len(hits) != 1 || len(hits[0].CharQuads) != 1 {
		t.Fatalf("search quads = %#v", hits)
	}
}

func TestExtractScaledGlyphMetrics(t *testing.T) {
	font := &displaylist.FontInfo{BaseFont: "Test", Ascent: 1056, Descent: -328}
	run := &displaylist.TextRun{Size: 1, Font: font, Glyphs: []displaylist.Glyph{{
		Text: "A", UnicodeOK: true,
		Origin:  displaylist.Point{X: 10, Y: 50},
		Advance: 0.5,
		Matrix:  displaylist.Matrix{20, 0, 0, -20, 10, 50},
	}}}
	p := Extract(&displaylist.Page{Width: 100, Height: 100, Items: []displaylist.Item{run}}, Options{})
	if len(p.Chars) != 1 {
		t.Fatalf("chars = %#v", p.Chars)
	}
	c := p.Chars[0]
	if math.Abs(c.Size-20) > 0.001 || math.Abs(c.Advance-10) > 0.001 {
		t.Fatalf("size/advance = %v/%v", c.Size, c.Advance)
	}
	if math.Abs(c.Bounds.Y0-(50-21.12)) > 0.001 || math.Abs(c.Bounds.Y1-(50+6.56)) > 0.001 {
		t.Fatalf("bounds = %+v", c.Bounds)
	}
	if math.Abs(c.Bounds.X1-20) > 0.001 {
		t.Fatalf("x bounds = %+v", c.Bounds)
	}
}
