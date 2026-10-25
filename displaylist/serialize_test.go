package displaylist

import "testing"

func TestVersion(t *testing.T) {
	if Version.Major != 2 || Version.Minor != 0 {
		t.Fatalf("version = %v", Version)
	}
	if !Compatible(2) || Compatible(1) || Compatible(0) {
		t.Fatal("compatibility")
	}
	if Version.String() != "2.0" {
		t.Fatalf("string = %s", Version.String())
	}
}

func TestCanonicalHashStable(t *testing.T) {
	p := &Page{
		Index: 0, Width: 200, Height: 100,
		MediaBox: Rect{0, 0, 200, 100},
		CropBox:  Rect{0, 0, 200, 100},
		Items: []Item{
			&TextRun{Common: Common{Z: 1, Clip: -1, Alpha: 1, ID: 1}, Size: 12, Glyphs: []Glyph{{Text: "Hi", UnicodeOK: true}}},
		},
	}
	a, err := HashPage(p)
	if err != nil {
		t.Fatal(err)
	}
	b, err := HashPage(p)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("hash unstable: %s != %s", a, b)
	}
	q := *p
	q.Width = 201
	c, _ := HashPage(&q)
	if c == a {
		t.Fatal("hash did not change with width")
	}
}

func TestEmptyPageHash(t *testing.T) {
	p := &Page{Index: 0, Width: 1, Height: 1}
	if _, err := CanonicalJSON(p); err != nil {
		t.Fatal(err)
	}
}

func TestItemBase(t *testing.T) {
	items := []Item{
		&TextRun{}, &Path{}, &Image{}, &Shading{}, &Group{},
	}
	for _, it := range items {
		if it.Base() == nil {
			t.Fatalf("%T has nil Base", it)
		}
	}
}
