package text

import (
	"testing"

	"github.com/F31/go-pdf/displaylist"
)

func searchPage() *Page {
	font := &displaylist.FontInfo{BaseFont: "Test"}
	glyphs := func(x float64, s string) []displaylist.Glyph {
		var gs []displaylist.Glyph
		for _, r := range s {
			gs = append(gs, displaylist.Glyph{Text: string(r), UnicodeOK: true, Origin: displaylist.Point{X: x, Y: 100}, Advance: 5})
			x += 5
		}
		return gs
	}
	dl := &displaylist.Page{Width: 200, Height: 200}
	dl.Items = []displaylist.Item{
		&displaylist.TextRun{Font: font, Size: 10, Glyphs: glyphs(10, "Hello")},
		&displaylist.TextRun{Font: font, Size: 10, Glyphs: glyphs(70, "World")},
	}
	return Extract(dl, Options{})
}

func TestSearch(t *testing.T) {
	p := searchPage()
	if hits := p.Search("hello", SearchOptions{}); len(hits) != 1 || hits[0].Text != "Hello" {
		t.Fatalf("hello: %#v", hits)
	}
	if hits := p.Search("hello world", SearchOptions{}); len(hits) != 1 {
		t.Fatalf("phrase: %#v", hits)
	}
	if hits := p.Search("HELLO", SearchOptions{CaseSensitive: true}); len(hits) != 0 {
		t.Fatalf("case: %#v", hits)
	}
	if hits := p.Search("ell", SearchOptions{WholeWord: true}); len(hits) != 0 {
		t.Fatalf("whole: %#v", hits)
	}
	if hits := p.Search("World", SearchOptions{WholeWord: true}); len(hits) != 1 || hits[0].Bounds.X0 < 60 {
		t.Fatalf("whole hit: %#v", hits)
	}
}

func TestVerticalTextExtraction(t *testing.T) {
	font := &displaylist.FontInfo{BaseFont: "Test"}
	dl := &displaylist.Page{Width: 100, Height: 100}
	dl.Items = []displaylist.Item{&displaylist.TextRun{
		Font: font, Size: 10, Vertical: true,
		Glyphs: []displaylist.Glyph{
			{Text: "A", UnicodeOK: true, Origin: displaylist.Point{X: 50, Y: 10}, Advance: 20},
			{Text: "B", UnicodeOK: true, Origin: displaylist.Point{X: 50, Y: 30}, Advance: 20},
		},
	}}
	p := Extract(dl, Options{})
	if !p.Vertical || p.Text != "AB" || len(p.Lines) != 1 {
		t.Fatalf("page %#v", p)
	}
}
