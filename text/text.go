// Package text builds a simple character, word and line index from a display
// list. It is intentionally independent of the PDF and render packages.
package text

import (
	"math"
	"sort"
	"strings"

	"github.com/F31/go-pdf/displaylist"
)

// Options controls word and line segmentation. Zero values use defaults.
type Options struct {
	SpaceScale       float64
	LineScale        float64
	IncludeInvisible bool
}

// Char is one decoded glyph or one ActualText span.
type Char struct {
	Text       string
	Origin     displaylist.Point
	Advance    float64
	Size       float64
	Bounds     displaylist.Rect
	Font       *displaylist.FontInfo
	UnicodeOK  bool
	Vertical   bool
	ActualText string
}

// Word is a run of chars with no significant gap or whitespace.
type Word struct {
	Text   string
	Chars  []Char
	Bounds displaylist.Rect
}

// Line is a baseline-aligned sequence of words.
type Line struct {
	Text   string
	Words  []Word
	Bounds displaylist.Rect
	Y      float64
}

// Page is the extracted text order for one page.
type Page struct {
	Index    int
	Vertical bool
	Chars    []Char
	Words    []Word
	Lines    []Line
	Text     string
	Bounds   displaylist.Rect
}

// Extract groups display-list text into chars, words and lines.
func Extract(dl *displaylist.Page, opts Options) *Page {
	if dl == nil {
		return &Page{}
	}
	if opts.SpaceScale <= 0 {
		opts.SpaceScale = 0.25
	}
	if opts.LineScale <= 0 {
		opts.LineScale = 0.5
	}
	out := &Page{Index: dl.Index}
	for _, it := range dl.Items {
		tr, ok := it.(*displaylist.TextRun)
		if !ok {
			continue
		}
		if tr.Flags&displaylist.FlagFromType3 != 0 {
			continue
		}
		if tr.TextFlags&displaylist.TextInvisible != 0 && !opts.IncludeInvisible {
			continue
		}
		if len(tr.Glyphs) == 0 {
			continue
		}
		if tr.ActualText != "" {
			g := tr.Glyphs[0]
			adv := tr.Bounds.X1 - tr.Bounds.X0
			if tr.Vertical {
				adv = tr.Bounds.Y1 - tr.Bounds.Y0
			}
			out.Chars = append(out.Chars, Char{
				Text: tr.ActualText, Origin: g.Origin, Advance: adv,
				Size: tr.Size, Bounds: tr.Bounds, Font: tr.Font, UnicodeOK: true,
				Vertical: tr.Vertical, ActualText: tr.ActualText,
			})
			continue
		}
		for _, g := range tr.Glyphs {
			b := displaylist.Rect{
				X0: g.Origin.X, Y0: g.Origin.Y - tr.Size,
				X1: g.Origin.X + g.Advance, Y1: g.Origin.Y + tr.Size*0.2,
			}
			if tr.Vertical {
				b = displaylist.Rect{
					X0: g.Origin.X - tr.Size*0.5, Y0: g.Origin.Y - tr.Size,
					X1: g.Origin.X + tr.Size*0.5, Y1: g.Origin.Y + tr.Size*0.2,
				}
			}
			out.Chars = append(out.Chars, Char{
				Text: g.Text, Origin: g.Origin, Advance: g.Advance, Size: tr.Size,
				Bounds: b, Font: tr.Font, UnicodeOK: g.UnicodeOK, Vertical: tr.Vertical,
			})
		}
	}
	if len(out.Chars) > 0 && allVertical(out.Chars) {
		out.Vertical = true
		sortVerticalChars(out.Chars)
		out.Lines = buildVerticalLines(out.Chars, opts)
	} else {
		out.Chars = sortChars(out.Chars)
		out.Lines = buildLines(out.Chars, opts)
	}
	for _, ln := range out.Lines {
		out.Words = append(out.Words, ln.Words...)
	}
	var lines []string
	for _, ln := range out.Lines {
		lines = append(lines, ln.Text)
	}
	out.Text = strings.Join(lines, "\n")
	out.Bounds = unionBounds(out.Chars)
	return out
}

func allVertical(chars []Char) bool {
	for _, c := range chars {
		if !c.Vertical {
			return false
		}
	}
	return true
}

func sortVerticalChars(chars []Char) {
	sort.SliceStable(chars, func(i, j int) bool {
		if math.Abs(chars[i].Origin.X-chars[j].Origin.X) > 0.01 {
			return chars[i].Origin.X < chars[j].Origin.X
		}
		return chars[i].Origin.Y < chars[j].Origin.Y
	})
}

func buildVerticalLines(chars []Char, opts Options) []Line {
	var lines []Line
	for i := 0; i < len(chars); {
		j := i + 1
		for j < len(chars) && math.Abs(chars[j].Origin.X-chars[i].Origin.X) <= math.Max(chars[i].Size, chars[j].Size)*0.5 {
			j++
		}
		col := chars[i:j]
		sort.SliceStable(col, func(a, b int) bool { return col[a].Origin.Y < col[b].Origin.Y })
		line := Line{Y: col[0].Origin.Y}
		line.Words = buildVerticalWords(col, opts)
		for _, w := range line.Words {
			line.Text += w.Text
		}
		line.Bounds = unionBounds(col)
		lines = append(lines, line)
		i = j
	}
	return lines
}

func buildVerticalWords(chars []Char, opts Options) []Word {
	var words []Word
	var cur []Char
	flush := func() {
		if len(cur) == 0 {
			return
		}
		var b strings.Builder
		for _, c := range cur {
			b.WriteString(c.Text)
		}
		words = append(words, Word{Text: b.String(), Chars: cur, Bounds: unionBounds(cur)})
		cur = nil
	}
	for _, c := range chars {
		if c.Text == "" || strings.TrimSpace(c.Text) == "" {
			flush()
			continue
		}
		if len(cur) > 0 {
			prev := cur[len(cur)-1]
			gap := c.Origin.Y - (prev.Origin.Y + prev.Advance)
			tol := math.Max(prev.Size, c.Size) * opts.SpaceScale
			if tol <= 0 {
				tol = 1
			}
			if gap > tol {
				flush()
			}
		}
		cur = append(cur, c)
	}
	flush()
	return words
}

func sortChars(chars []Char) []Char {
	sort.SliceStable(chars, func(i, j int) bool {
		if math.Abs(chars[i].Origin.Y-chars[j].Origin.Y) > 0.01 {
			return chars[i].Origin.Y < chars[j].Origin.Y
		}
		return chars[i].Origin.X < chars[j].Origin.X
	})
	return chars
}

func buildLines(chars []Char, opts Options) []Line {
	if len(chars) == 0 {
		return nil
	}
	var lines []Line
	var cur []Char
	flush := func() {
		if len(cur) == 0 {
			return
		}
		sort.SliceStable(cur, func(i, j int) bool { return cur[i].Origin.X < cur[j].Origin.X })
		line := Line{Y: cur[0].Origin.Y}
		line.Words = buildWords(cur, opts)
		for _, w := range line.Words {
			if line.Text != "" {
				line.Text += " "
			}
			line.Text += w.Text
		}
		line.Bounds = unionBounds(cur)
		lines = append(lines, line)
		cur = nil
	}
	for _, c := range chars {
		if len(cur) > 0 {
			tol := math.Max(cur[0].Size, c.Size) * opts.LineScale
			if tol <= 0 {
				tol = 0.5
			}
			if math.Abs(c.Origin.Y-cur[0].Origin.Y) > tol {
				flush()
			}
		}
		cur = append(cur, c)
	}
	flush()
	return lines
}

func buildWords(chars []Char, opts Options) []Word {
	var words []Word
	var cur []Char
	flush := func() {
		if len(cur) == 0 {
			return
		}
		var b strings.Builder
		for _, c := range cur {
			b.WriteString(c.Text)
		}
		words = append(words, Word{Text: b.String(), Chars: cur, Bounds: unionBounds(cur)})
		cur = nil
	}
	for _, c := range chars {
		if c.Text == "" || strings.TrimSpace(c.Text) == "" {
			flush()
			continue
		}
		if len(cur) > 0 {
			prev := cur[len(cur)-1]
			gap := c.Origin.X - (prev.Origin.X + prev.Advance)
			tol := math.Max(prev.Size, c.Size) * opts.SpaceScale
			if tol <= 0 {
				tol = 1
			}
			if gap > tol {
				flush()
			}
		}
		cur = append(cur, c)
	}
	flush()
	return words
}

func unionBounds(chars []Char) displaylist.Rect {
	if len(chars) == 0 {
		return displaylist.Rect{}
	}
	r := chars[0].Bounds
	for _, c := range chars[1:] {
		r = union(r, c.Bounds)
	}
	return r
}

func union(a, b displaylist.Rect) displaylist.Rect {
	return displaylist.Rect{
		X0: math.Min(a.X0, b.X0), Y0: math.Min(a.Y0, b.Y0),
		X1: math.Max(a.X1, b.X1), Y1: math.Max(a.Y1, b.Y1),
	}
}
