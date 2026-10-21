package displaylist_test

import (
	"testing"

	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/internal/testkit"
)

func TestCanonicalGolden(t *testing.T) {
	p := &displaylist.Page{
		Index: 0, Width: 200, Height: 100, Rotate: 0,
		MediaBox: displaylist.Rect{X0: 0, Y0: 0, X1: 200, Y1: 100},
		CropBox:  displaylist.Rect{X0: 0, Y0: 0, X1: 200, Y1: 100},
		Clips:    []displaylist.Clip{},
		Items: []displaylist.Item{
			&displaylist.TextRun{
				Common: displaylist.Common{Z: 0, Clip: -1, Alpha: 1, ID: 1},
				Size:   12,
				Fill:   displaylist.Paint{Kind: displaylist.PaintSolid, RGBA: [4]float64{0, 0, 0, 1}},
				Glyphs: []displaylist.Glyph{{Text: "Hi", UnicodeOK: true, Code: 72}},
			},
		},
	}
	b, err := displaylist.CanonicalJSON(p)
	if err != nil {
		t.Fatal(err)
	}
	testkit.Golden(t, "../testdata/goldens/displaylist-page.json", b)
}
