// Package text builds character, word and line indexes from a display list. It
// is intentionally independent of the PDF and render packages. Horizontal text
// is split into line segments and ordered by detected columns. Vertical text
// defaults to right-to-left columns. Pure RTL and
// simple mixed-direction lines are reordered into logical order at word/run
// granularity. This is a screen-reading heuristic, not a full UBA/layout engine.
package text

import (
	"math"
	"sort"
	"strings"

	"github.com/F31/go-pdf/displaylist"
	"golang.org/x/text/unicode/bidi"
	"golang.org/x/text/unicode/norm"
)

// Options controls word and line segmentation. Zero values use defaults.
type Normalization uint8

const (
	NormalizationNone Normalization = iota
	NormalizationNFC
	NormalizationNFKC
)

func (n Normalization) apply(s string) string {
	switch n {
	case NormalizationNFC:
		return norm.NFC.String(s)
	case NormalizationNFKC:
		return norm.NFKC.String(s)
	default:
		return s
	}
}

// VerticalOrder selects the reading direction of vertical columns. CJK
// vertical text conventionally advances right-to-left.
type VerticalOrder uint8

const (
	VerticalRTL VerticalOrder = iota
	VerticalLTR
)

type Options struct {
	SpaceScale       float64
	LineScale        float64
	IncludeInvisible bool
	// Normalization is applied per word; Search applies the same form to its
	// query. Defaults to NormalizationNone.
	Normalization Normalization
	// StructOrder lists tagged-PDF structure IDs in reading order. When set,
	// lines with known IDs are ordered by this sequence before column order.
	StructOrder []int
	// StructInfo maps structure IDs to semantic metadata for Blocks.
	StructInfo map[int]StructInfo
	// VerticalOrder selects vertical column order. The zero value is
	// VerticalRTL.
	VerticalOrder VerticalOrder
}

type StructInfo struct {
	Type  string
	Title string
	Alt   string
	Lang  string
}

// Direction is the detected base direction of a line.
type Direction uint8

const (
	DirectionLTR Direction = iota
	DirectionRTL
	DirectionMixed
	DirectionNeutral
)

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
	StructRef  int
	Rotation   float64
	Quad       [4]displaylist.Point
}

// Word is a run of chars with no significant gap or whitespace.
type Word struct {
	Text   string
	Chars  []Char
	Bounds displaylist.Rect
}

// Line is a baseline-aligned sequence of words.
type Line struct {
	Text          string
	Words         []Word
	Bounds        displaylist.Rect
	Y             float64
	Column        int
	Block         int
	StructRef     int
	Direction     Direction
	BaseDirection Direction
	Rotation      float64
}

// Block groups consecutive lines in the same column or tagged structure
// element. Blocks are a reading-order aid, not a layout tree.
type Block struct {
	Text      string
	Lines     []Line
	Bounds    displaylist.Rect
	Column    int
	StructRef int
	Type      string
	Title     string
	Alt       string
	Lang      string
}

// Page is the extracted text order for one page.
type Page struct {
	Index         int
	Vertical      bool
	Chars         []Char
	Words         []Word
	Lines         []Line
	Blocks        []Block
	Text          string
	Bounds        displaylist.Rect
	Columns       int
	Normalization Normalization
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
	out := &Page{Index: dl.Index, Normalization: opts.Normalization}
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
			size := glyphSize(g.Matrix, tr.Size)
			if tr.Size > size {
				size = tr.Size
			}
			adv := tr.Bounds.X1 - tr.Bounds.X0
			if tr.Vertical {
				adv = tr.Bounds.Y1 - tr.Bounds.Y0
			}
			rotation := glyphRotation(g.Matrix)
			quad := glyphQuad(g.Origin, adv, size, g.Matrix)
			if tr.Vertical {
				rotation = 90
				quad = verticalQuad(g.Origin, adv, size)
			}
			out.Chars = append(out.Chars, Char{
				Text: opts.Normalization.apply(tr.ActualText), Origin: g.Origin, Advance: adv,
				Size: size, Bounds: tr.Bounds, Font: tr.Font, UnicodeOK: true,
				Vertical: tr.Vertical, ActualText: tr.ActualText, StructRef: tr.StructRef,
				Rotation: rotation, Quad: quad,
			})
			continue
		}
		for _, g := range tr.Glyphs {
			size := glyphSize(g.Matrix, tr.Size)
			if tr.Size > size {
				size = tr.Size
			}
			scale := 1.0
			if tr.Size > 0 {
				scale = size / tr.Size
			}
			adv := g.Advance * scale
			ascent, descent := fontAscentDescent(tr.Font, size)
			b := displaylist.Rect{
				X0: g.Origin.X, Y0: g.Origin.Y - ascent,
				X1: g.Origin.X + adv, Y1: g.Origin.Y - descent,
			}
			if tr.Vertical {
				b = displaylist.Rect{
					X0: g.Origin.X - size*0.5, Y0: g.Origin.Y - size,
					X1: g.Origin.X + size*0.5, Y1: g.Origin.Y + size*0.2,
				}
			}
			rotation := glyphRotation(g.Matrix)
			quad := glyphQuad(g.Origin, adv, size, g.Matrix)
			if tr.Vertical {
				rotation = 90
				quad = verticalQuad(g.Origin, adv, size)
			}
			out.Chars = append(out.Chars, Char{
				Text: opts.Normalization.apply(g.Text), Origin: g.Origin, Advance: adv, Size: size,
				Bounds: b, Font: tr.Font, UnicodeOK: g.UnicodeOK, Vertical: tr.Vertical,
				StructRef: tr.StructRef, Rotation: rotation, Quad: quad,
			})
		}
	}
	if len(out.Chars) > 0 && allVertical(out.Chars) {
		out.Vertical = true
		sortVerticalChars(out.Chars, opts.VerticalOrder)
		out.Lines = buildVerticalLines(out.Chars, opts)
		applyStructOrder(out.Lines, opts.StructOrder)
		for i := range out.Lines {
			out.Lines[i].Direction = lineDirection(out.Lines[i].Text)
			out.Lines[i].BaseDirection = baseDirection(out.Lines[i].Text)
		}
		out.Columns = countColumns(out.Lines)
		out.Blocks, out.Lines = buildBlocks(out.Lines, opts)
	} else {
		horizontal, rotated := splitRotatedChars(out.Chars)
		if len(horizontal) > 0 {
			sortChars(horizontal)
			out.Lines = orderLines(buildLines(horizontal, opts, dl.Width), dl.Width)
		}
		if len(rotated) > 0 {
			out.Lines = append(out.Lines, buildRotatedLines(rotated, opts)...)
		}
		applyStructOrder(out.Lines, opts.StructOrder)
		applyBidi(out.Lines)
		out.Columns = countColumns(out.Lines)
		out.Blocks, out.Lines = buildBlocks(out.Lines, opts)
	}
	for _, ln := range out.Lines {
		out.Words = append(out.Words, ln.Words...)
	}
	// Rebuild Chars in reading order. Vertical/bidi line assembly may differ
	// from geometric X/Y order, and Search must follow the same sequence.
	out.Chars = out.Chars[:0]
	for _, ln := range out.Lines {
		for _, w := range ln.Words {
			for _, c := range w.Chars {
				if c.Text != "" {
					out.Chars = append(out.Chars, c)
				}
			}
		}
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

func sortVerticalChars(chars []Char, order VerticalOrder) {
	sort.SliceStable(chars, func(i, j int) bool {
		if math.Abs(chars[i].Origin.X-chars[j].Origin.X) > 0.01 {
			if order == VerticalLTR {
				return chars[i].Origin.X < chars[j].Origin.X
			}
			return chars[i].Origin.X > chars[j].Origin.X
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
		line := Line{Y: col[0].Origin.Y, StructRef: col[0].StructRef}
		line.Words = buildVerticalWords(col, opts)
		for _, w := range line.Words {
			line.Text += w.Text
		}
		line.Bounds = unionBounds(col)
		line.Rotation = col[0].Rotation
		line.Column = len(lines)
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
		w := Word{Text: b.String(), Chars: cur, Bounds: unionBounds(cur)}
		normalizeWord(&w, opts.Normalization)
		words = append(words, w)
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

func splitRotatedChars(chars []Char) (horizontal, rotated []Char) {
	for _, c := range chars {
		a := normalizeRotation(c.Rotation)
		if a < 0.5 || a > 359.5 {
			horizontal = append(horizontal, c)
		} else {
			rotated = append(rotated, c)
		}
	}
	return horizontal, rotated
}

func normalizeRotation(a float64) float64 {
	a = math.Mod(a, 360)
	if a < 0 {
		a += 360
	}
	return a
}

type rotatedChar struct {
	c Char
	u float64
	v float64
}

// buildRotatedLines groups glyphs by their baseline normal and orders them
// along the rotated baseline. Without this, 90/270 degree text turns into one
// line per glyph because the ordinary group is keyed only by page Y.
func buildRotatedLines(chars []Char, opts Options) []Line {
	groups := make(map[int][]Char)
	var keys []int
	for _, c := range chars {
		key := int(math.Round(normalizeRotation(c.Rotation))) % 360
		if _, ok := groups[key]; !ok {
			keys = append(keys, key)
		}
		groups[key] = append(groups[key], c)
	}
	sort.Ints(keys)
	var lines []Line
	for _, key := range keys {
		lines = append(lines, buildRotatedGroupLines(groups[key], float64(key), opts)...)
	}
	sort.SliceStable(lines, func(i, j int) bool {
		if lines[i].Bounds.Y0 != lines[j].Bounds.Y0 {
			return lines[i].Bounds.Y0 < lines[j].Bounds.Y0
		}
		return lines[i].Bounds.X0 < lines[j].Bounds.X0
	})
	return lines
}

func buildRotatedGroupLines(chars []Char, angle float64, opts Options) []Line {
	rad := angle * math.Pi / 180
	cos, sin := math.Cos(rad), math.Sin(rad)
	items := make([]rotatedChar, len(chars))
	for i, c := range chars {
		items[i] = rotatedChar{
			c: c,
			u: c.Origin.X*cos + c.Origin.Y*sin,
			v: -c.Origin.X*sin + c.Origin.Y*cos,
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if math.Abs(items[i].v-items[j].v) > 0.01 {
			return items[i].v < items[j].v
		}
		return items[i].u < items[j].u
	})
	var lines []Line
	for i := 0; i < len(items); {
		j := i + 1
		tol := math.Max(items[i].c.Size, items[j-1].c.Size) * opts.LineScale
		if tol <= 0 {
			tol = 0.5
		}
		for j < len(items) {
			if math.Abs(items[j].v-items[i].v) > tol {
				break
			}
			j++
		}
		seg := items[i:j]
		sort.SliceStable(seg, func(a, b int) bool { return seg[a].u < seg[b].u })
		line := Line{Y: seg[0].c.Origin.Y, StructRef: seg[0].c.StructRef, Rotation: angle, Bounds: seg[0].c.Bounds}
		line.Words = buildRotatedWords(seg, opts)
		for _, w := range line.Words {
			if line.Text != "" {
				line.Text += " "
			}
			line.Text += w.Text
		}
		for _, rc := range seg[1:] {
			line.Bounds = union(line.Bounds, rc.c.Bounds)
		}
		lines = append(lines, line)
		i = j
	}
	return lines
}

func buildRotatedWords(items []rotatedChar, opts Options) []Word {
	var words []Word
	var cur []rotatedChar
	flush := func() {
		if len(cur) == 0 {
			return
		}
		var b strings.Builder
		chars := make([]Char, len(cur))
		for i, rc := range cur {
			b.WriteString(rc.c.Text)
			chars[i] = rc.c
		}
		w := Word{Text: b.String(), Chars: chars, Bounds: unionBounds(chars)}
		normalizeWord(&w, opts.Normalization)
		words = append(words, w)
		cur = nil
	}
	for _, rc := range items {
		if rc.c.Text == "" || strings.TrimSpace(rc.c.Text) == "" {
			flush()
			continue
		}
		if len(cur) > 0 {
			prev := cur[len(cur)-1]
			gap := rc.u - (prev.u + prev.c.Advance)
			tol := math.Max(prev.c.Size, rc.c.Size) * opts.SpaceScale
			if tol <= 0 {
				tol = 1
			}
			if gap > tol {
				flush()
			}
		}
		cur = append(cur, rc)
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

func buildLines(chars []Char, opts Options, pageWidth float64) []Line {
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
		for _, seg := range splitLine(cur, opts, pageWidth) {
			line := Line{Y: seg[0].Origin.Y, StructRef: seg[0].StructRef}
			line.Words = buildWords(seg, opts)
			for _, w := range line.Words {
				if line.Text != "" {
					line.Text += " "
				}
				line.Text += w.Text
			}
			line.Bounds = unionBounds(seg)
			line.Rotation = seg[0].Rotation
			lines = append(lines, line)
		}
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

func splitLine(chars []Char, opts Options, pageWidth float64) [][]Char {
	if len(chars) < 2 {
		return [][]Char{chars}
	}
	var out [][]Char
	start := 0
	for i := 1; i < len(chars); i++ {
		prev, cur := chars[i-1], chars[i]
		gap := cur.Bounds.X0 - prev.Bounds.X1
		size := math.Max(prev.Size, cur.Size)
		tol := size * 3
		if pageWidth > 0 && pageWidth*0.05 > tol {
			tol = pageWidth * 0.05
		}
		if gap > tol || prev.StructRef != cur.StructRef {
			out = append(out, chars[start:i])
			start = i
		}
	}
	out = append(out, chars[start:])
	return out
}

func orderLines(lines []Line, pageWidth float64) []Line {
	if len(lines) < 2 {
		return lines
	}
	fullWidth := 0.6 * pageWidth
	var full, normal []Line
	for _, line := range lines {
		if fullWidth > 0 && line.Bounds.X1-line.Bounds.X0 >= fullWidth {
			full = append(full, line)
		} else {
			normal = append(normal, line)
		}
	}
	sort.SliceStable(full, func(i, j int) bool {
		if full[i].Y != full[j].Y {
			return full[i].Y < full[j].Y
		}
		return full[i].Bounds.X0 < full[j].Bounds.X0
	})
	order := append([]Line(nil), full...)
	if len(normal) == 0 {
		return order
	}
	sort.SliceStable(normal, func(i, j int) bool {
		if normal[i].Bounds.X0 != normal[j].Bounds.X0 {
			return normal[i].Bounds.X0 < normal[j].Bounds.X0
		}
		return normal[i].Y < normal[j].Y
	})
	var columns [][]Line
	tol := 0.02 * pageWidth
	if tol <= 0 {
		tol = 1
	}
	for _, line := range normal {
		if len(columns) == 0 {
			columns = append(columns, []Line{line})
			continue
		}
		last := columns[len(columns)-1]
		maxX := last[0].Bounds.X1
		for _, l := range last[1:] {
			if l.Bounds.X1 > maxX {
				maxX = l.Bounds.X1
			}
		}
		if line.Bounds.X0 <= maxX+tol {
			columns[len(columns)-1] = append(columns[len(columns)-1], line)
		} else {
			columns = append(columns, []Line{line})
		}
	}
	for ci := range columns {
		col := columns[ci]
		sort.SliceStable(col, func(i, j int) bool {
			if col[i].Y != col[j].Y {
				return col[i].Y < col[j].Y
			}
			return col[i].Bounds.X0 < col[j].Bounds.X0
		})
		for i := range col {
			col[i].Column = ci
		}
		order = append(order, col...)
	}
	return order
}

func countColumns(lines []Line) int {
	max := -1
	for _, line := range lines {
		if line.Column > max {
			max = line.Column
		}
	}
	return max + 1
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
		w := Word{Text: b.String(), Chars: cur, Bounds: unionBounds(cur)}
		normalizeWord(&w, opts.Normalization)
		words = append(words, w)
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

func applyBidi(lines []Line) {
	for i := range lines {
		line := &lines[i]
		line.Direction = lineDirection(line.Text)
		line.BaseDirection = baseDirection(line.Text)
		if line.Direction != DirectionRTL && line.Direction != DirectionMixed {
			continue
		}
		if line.BaseDirection == DirectionRTL {
			for a, b := 0, len(line.Words)-1; a < b; a, b = a+1, b-1 {
				line.Words[a], line.Words[b] = line.Words[b], line.Words[a]
			}
		}
		for w := range line.Words {
			if lineDirection(line.Words[w].Text) == DirectionRTL {
				line.Words[w].Text = reverseRunes(line.Words[w].Text)
				for a, b := 0, len(line.Words[w].Chars)-1; a < b; a, b = a+1, b-1 {
					line.Words[w].Chars[a], line.Words[w].Chars[b] = line.Words[w].Chars[b], line.Words[w].Chars[a]
				}
			}
		}
		var parts []string
		for _, w := range line.Words {
			parts = append(parts, w.Text)
		}
		line.Text = strings.Join(parts, " ")
	}
}

func baseDirection(text string) Direction {
	for len(text) > 0 {
		p, size := bidi.LookupString(text)
		if size <= 0 {
			return DirectionNeutral
		}
		switch p.Class() {
		case bidi.L:
			return DirectionLTR
		case bidi.R, bidi.AL:
			return DirectionRTL
		}
		text = text[size:]
	}
	return DirectionNeutral
}

func lineDirection(text string) Direction {
	var ltr, rtl bool
	for len(text) > 0 {
		p, size := bidi.LookupString(text)
		if size <= 0 {
			break
		}
		switch p.Class() {
		case bidi.L:
			ltr = true
		case bidi.R, bidi.AL:
			rtl = true
		}
		text = text[size:]
	}
	switch {
	case ltr && rtl:
		return DirectionMixed
	case rtl:
		return DirectionRTL
	case ltr:
		return DirectionLTR
	default:
		return DirectionNeutral
	}
}

func reverseRunes(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

func applyStructOrder(lines []Line, order []int) {
	if len(order) == 0 || len(lines) < 2 {
		return
	}
	rank := make(map[int]int, len(order))
	for i, id := range order {
		if _, ok := rank[id]; !ok {
			rank[id] = i
		}
	}
	sort.SliceStable(lines, func(i, j int) bool {
		ri, iok := rank[lines[i].StructRef]
		rj, jok := rank[lines[j].StructRef]
		if iok && jok && ri != rj {
			return ri < rj
		}
		return false
	})
}

func buildBlocks(lines []Line, opts Options) ([]Block, []Line) {
	var blocks []Block
	for i := 0; i < len(lines); {
		first := lines[i]
		j := i + 1
		for j < len(lines) && lines[j].Column == first.Column && lines[j].StructRef == first.StructRef {
			j++
		}
		info := opts.StructInfo[first.StructRef]
		b := Block{
			Lines: append([]Line(nil), lines[i:j]...), Column: first.Column, StructRef: first.StructRef,
			Type: info.Type, Title: info.Title, Alt: info.Alt, Lang: info.Lang,
		}
		for k := range b.Lines {
			b.Lines[k].Block = len(blocks)
			if b.Text != "" {
				b.Text += "\n"
			}
			b.Text += b.Lines[k].Text
			b.Bounds = union(b.Bounds, b.Lines[k].Bounds)
			lines[i+k].Block = len(blocks)
		}
		blocks = append(blocks, b)
		i = j
	}
	return blocks, lines
}

func normalizeWord(w *Word, n Normalization) {
	if n == NormalizationNone || w == nil || len(w.Chars) == 0 {
		return
	}
	normalized := n.apply(w.Text)
	if normalized == w.Text {
		return
	}
	w.Text = normalized
	w.Chars[0].Text = normalized
	for i := 1; i < len(w.Chars); i++ {
		w.Chars[i].Text = ""
	}
}

func glyphRotation(m displaylist.Matrix) float64 {
	if m[0] == 0 && m[1] == 0 {
		return 0
	}
	return math.Atan2(m[1], m[0]) * 180 / math.Pi
}

func fontAscentDescent(font *displaylist.FontInfo, size float64) (ascent, descent float64) {
	ascent, descent = size, -size*0.2
	if font == nil {
		return ascent, descent
	}
	if font.Ascent > 0 {
		ascent = font.Ascent * size / 1000
	}
	if font.Descent < 0 {
		descent = font.Descent * size / 1000
	}
	return ascent, descent
}

func glyphSize(m displaylist.Matrix, fallback float64) float64 {
	if s := math.Hypot(m[2], m[3]); s > 0 {
		return s
	}
	return fallback
}

func glyphQuad(origin displaylist.Point, advance, size float64, m displaylist.Matrix) [4]displaylist.Point {
	dx, dy := m[0], m[1]
	if dx == 0 && dy == 0 {
		dx, dy = 1, 0
	}
	l := math.Hypot(dx, dy)
	dx, dy = dx/l, dy/l
	nx, ny := -dy, dx
	// Match the unrotated bounds convention: descender below origin, small
	// ascender above, then extend by the advance along the baseline.
	p0 := displaylist.Point{X: origin.X - nx*size, Y: origin.Y - ny*size}
	p1 := displaylist.Point{X: p0.X + dx*advance, Y: p0.Y + dy*advance}
	p2 := displaylist.Point{X: p1.X + nx*(size*1.2), Y: p1.Y + ny*(size*1.2)}
	p3 := displaylist.Point{X: origin.X + nx*(size*0.2), Y: origin.Y + ny*(size*0.2)}
	return [4]displaylist.Point{p0, p1, p2, p3}
}

func verticalQuad(origin displaylist.Point, advance, size float64) [4]displaylist.Point {
	half := size * 0.5
	p0 := displaylist.Point{X: origin.X - half, Y: origin.Y}
	p1 := displaylist.Point{X: origin.X - half, Y: origin.Y + advance}
	p2 := displaylist.Point{X: origin.X + half, Y: origin.Y + advance}
	p3 := displaylist.Point{X: origin.X + half, Y: origin.Y}
	return [4]displaylist.Point{p0, p1, p2, p3}
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
