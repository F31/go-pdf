package text

import "github.com/F31/go-pdf/displaylist"

// Selection is a rune range over Page.Text mapped back to character bounds.
type Selection struct {
	Text      string
	Bounds    displaylist.Rect
	Quads     []displaylist.Rect
	CharQuads [][4]displaylist.Point
	Chars     []Char
}

// Select returns the selection for rune offsets [start,end). Offsets are
// clamped; start greater than end is swapped. Spaces and line breaks are kept
// in Text but do not contribute quads.
func (p *Page) Select(start, end int) Selection {
	if p == nil {
		return Selection{}
	}
	if start < 0 {
		start = 0
	}
	runes, chars, ids, lineAt := p.selectionRunes()
	if end > len(runes) {
		end = len(runes)
	}
	if end < 0 {
		end = 0
	}
	if start > end {
		start, end = end, start
	}
	if start >= len(runes) || end <= start {
		return Selection{}
	}
	sel := Selection{Text: string(runes[start:end])}
	seen := map[int]bool{}
	lineQuads := map[int]displaylist.Rect{}
	var lineOrder []int
	for i := start; i < end; i++ {
		ci := ids[i]
		if ci < 0 || chars[i].Text == "" || seen[ci] {
			continue
		}
		seen[ci] = true
		sel.Chars = append(sel.Chars, chars[i])
		sel.CharQuads = append(sel.CharQuads, chars[i].Quad)
		if li := lineAt[i]; li >= 0 {
			q, ok := lineQuads[li]
			if !ok {
				lineOrder = append(lineOrder, li)
				lineQuads[li] = chars[i].Bounds
			} else {
				lineQuads[li] = union(q, chars[i].Bounds)
			}
		}
	}
	for _, li := range lineOrder {
		sel.Quads = append(sel.Quads, lineQuads[li])
	}
	sel.Bounds = unionBounds(sel.Chars)
	return sel
}

func (p *Page) selectionRunes() ([]rune, []Char, []int, []int) {
	var runes []rune
	var chars []Char
	var ids, lineAt []int
	nextID := 0
	for li, line := range p.Lines {
		if li > 0 {
			runes = append(runes, '\n')
			chars = append(chars, Char{})
			ids = append(ids, -1)
			lineAt = append(lineAt, -1)
		}
		for wi, word := range line.Words {
			if wi > 0 {
				runes = append(runes, ' ')
				chars = append(chars, Char{})
				ids = append(ids, -1)
				lineAt = append(lineAt, li)
			}
			for _, c := range word.Chars {
				id := nextID
				nextID++
				for _, r := range c.Text {
					runes = append(runes, r)
					chars = append(chars, c)
					ids = append(ids, id)
					lineAt = append(lineAt, li)
				}
			}
		}
	}
	return runes, chars, ids, lineAt
}
