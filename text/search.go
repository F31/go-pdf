package text

import (
	"strings"
	"unicode"

	"github.com/F31/go-pdf/displaylist"
)

// SearchOptions controls text matching.
type SearchOptions struct {
	CaseSensitive bool
	WholeWord     bool
}

// Hit is one search match with its page-space bounds.
type Hit struct {
	Text   string
	Bounds displaylist.Rect
	Chars  []Char
}

// Search finds query occurrences in the page text, including whitespace gaps
// between words.
func (p *Page) Search(query string, opts SearchOptions) []Hit {
	if p == nil || query == "" {
		return nil
	}
	runes, chars, positions := p.searchRunes()
	q := []rune(query)
	match := runes
	if !opts.CaseSensitive {
		for i := range q {
			q[i] = unicode.ToLower(q[i])
		}
		match = make([]rune, len(runes))
		for i := range runes {
			match[i] = unicode.ToLower(runes[i])
		}
	}
	var hits []Hit
	for i := 0; i+len(q) <= len(match); i++ {
		if !equalRunes(match[i:i+len(q)], q) {
			continue
		}
		if opts.WholeWord && !wordBoundary(match, i, i+len(q)) {
			continue
		}
		seen := make(map[int]bool)
		var hitChars []Char
		for k := i; k < i+len(q); k++ {
			pos := positions[k]
			if pos < 0 || seen[pos] {
				continue
			}
			seen[pos] = true
			hitChars = append(hitChars, chars[pos])
		}
		if len(hitChars) == 0 {
			continue
		}
		hits = append(hits, Hit{
			Text:   strings.TrimSpace(string(runes[i : i+len(q)])),
			Bounds: unionBounds(hitChars),
			Chars:  hitChars,
		})
	}
	return hits
}

func equalRunes(a, b []rune) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func wordBoundary(runes []rune, start, end int) bool {
	if start > 0 && isWordRune(runes[start-1]) && isWordRune(runes[start]) {
		return false
	}
	if end < len(runes) && isWordRune(runes[end-1]) && isWordRune(runes[end]) {
		return false
	}
	return true
}

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func (p *Page) searchRunes() ([]rune, []Char, []int) {
	var out []rune
	var chars []Char
	var positions []int
	appendChar := func(c Char) {
		pos := len(chars)
		chars = append(chars, c)
		for _, r := range c.Text {
			out = append(out, r)
			positions = append(positions, pos)
		}
	}
	appendSpace := func() {
		if len(out) == 0 || unicode.IsSpace(out[len(out)-1]) {
			return
		}
		out = append(out, ' ')
		positions = append(positions, -1)
	}
	if len(p.Lines) == 0 {
		for _, c := range p.Chars {
			appendChar(c)
		}
		return out, chars, positions
	}
	for li, line := range p.Lines {
		if li > 0 {
			appendSpace()
		}
		for wi, word := range line.Words {
			if wi > 0 {
				appendSpace()
			}
			for _, c := range word.Chars {
				appendChar(c)
			}
		}
	}
	return out, chars, positions
}
