package render

import (
	"sync"
	"unicode/utf8"

	"github.com/F31/go-pdf/displaylist"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/gomonobolditalic"
	"golang.org/x/image/font/gofont/gomonoitalic"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// builtinFontProvider supplies Go font fallback outlines for text runs that
// have no embedded GlyphSource, such as base-14 fonts. It is intended for
// Preview/preview-like output and does not claim source-font fidelity.
type builtinFontProvider struct{}

var (
	builtinOnce  sync.Once
	builtinFaces map[string]*sfnt.Font
	builtinErr   error

	defaultFontsOnce sync.Once
	defaultFonts     FontProvider
)

// BuiltinFonts returns the built-in Go font fallback. It selects sans-serif or
// monospaced faces and their bold/italic variants. Serif and CJK requests are
// substituted; system font discovery is not performed. Callers should install
// it through PageOptions.Fonts or Renderer Options when rendering text whose
// font program is not embedded.
func BuiltinFonts() FontProvider { return builtinFontProvider{} }

func loadBuiltinFaces() (map[string]*sfnt.Font, error) {
	builtinOnce.Do(func() {
		data := []struct {
			name string
			ttf  []byte
		}{
			{"regular", goregular.TTF},
			{"bold", gobold.TTF},
			{"italic", goitalic.TTF},
			{"bolditalic", gobolditalic.TTF},
			{"mono", gomono.TTF},
			{"monobold", gomonobold.TTF},
			{"monoitalic", gomonoitalic.TTF},
			{"monobolditalic", gomonobolditalic.TTF},
		}
		builtinFaces = make(map[string]*sfnt.Font, len(data))
		for _, f := range data {
			face, err := sfnt.Parse(f.ttf)
			if err != nil {
				builtinErr = err
				return
			}
			builtinFaces[f.name] = face
		}
	})
	return builtinFaces, builtinErr
}

func builtinFaceName(info *displaylist.FontInfo) string {
	weight := ""
	if info.Bold {
		weight = "bold"
	}
	slant := ""
	if info.Italic {
		slant = "italic"
	}
	if info.Mono {
		return "mono" + weight + slant
	}
	switch weight + slant {
	case "bold":
		return "bold"
	case "italic":
		return "italic"
	case "bolditalic":
		return "bolditalic"
	default:
		return "regular"
	}
}

func (builtinFontProvider) GlyphOutline(info *displaylist.FontInfo, glyph displaylist.Glyph) ([]displaylist.Segment, bool, error) {
	if info == nil || info.Kind == "Type3" {
		return nil, false, nil
	}
	r, ok := fallbackRune(glyph)
	if !ok {
		return nil, false, nil
	}
	faces, err := loadBuiltinFaces()
	if err != nil {
		return nil, false, err
	}
	face := faces[builtinFaceName(info)]
	if face == nil {
		face = faces["regular"]
	}
	if face == nil {
		return nil, false, nil
	}
	var buf sfnt.Buffer
	gid, err := face.GlyphIndex(&buf, r)
	if err != nil || gid == 0 {
		return nil, false, err
	}
	upem := int(face.UnitsPerEm())
	segs, err := face.LoadGlyph(&buf, gid, fixed.I(upem), nil)
	if err != nil {
		return nil, false, err
	}
	out := fallbackSegments(segs, upem)
	return out, len(out) > 0, nil
}

func fallbackRune(g displaylist.Glyph) (rune, bool) {
	if g.Text != "" {
		r, n := utf8.DecodeRuneInString(g.Text)
		if r != utf8.RuneError || n > 1 {
			return r, true
		}
	}
	if g.Code < 0x80 {
		return rune(g.Code), true
	}
	return 0, false
}

func fallbackSegments(in sfnt.Segments, upem int) []displaylist.Segment {
	if upem <= 0 || len(in) == 0 {
		return nil
	}
	scale := 1000 / float64(upem)
	conv := func(p fixed.Point26_6) displaylist.Point {
		x, y := float64(p.X)/64*scale, -float64(p.Y)/64*scale
		if x == 0 {
			x = 0
		}
		if y == 0 {
			y = 0
		}
		return displaylist.Point{X: x, Y: y}
	}
	out := make([]displaylist.Segment, 0, len(in)+4)
	var start, cur displaylist.Point
	open := false
	closeContour := func() {
		if open {
			out = append(out, displaylist.Segment{Kind: displaylist.SegmentClose, P0: cur, P3: start})
		}
		open = false
	}
	for _, s := range in {
		switch s.Op {
		case sfnt.SegmentOpMoveTo:
			closeContour()
			cur = conv(s.Args[0])
			start = cur
			out = append(out, displaylist.Segment{Kind: displaylist.SegmentMove, P0: cur})
			open = true
		case sfnt.SegmentOpLineTo:
			if !open {
				continue
			}
			next := conv(s.Args[0])
			out = append(out, displaylist.Segment{Kind: displaylist.SegmentLine, P0: cur, P3: next})
			cur = next
		case sfnt.SegmentOpQuadTo:
			if !open {
				continue
			}
			ctrl, next := conv(s.Args[0]), conv(s.Args[1])
			c1 := displaylist.Point{X: cur.X + 2*(ctrl.X-cur.X)/3, Y: cur.Y + 2*(ctrl.Y-cur.Y)/3}
			c2 := displaylist.Point{X: next.X + 2*(ctrl.X-next.X)/3, Y: next.Y + 2*(ctrl.Y-next.Y)/3}
			out = append(out, displaylist.Segment{Kind: displaylist.SegmentCubic, P0: cur, P1: c1, P2: c2, P3: next})
			cur = next
		case sfnt.SegmentOpCubeTo:
			if !open {
				continue
			}
			c1, c2, next := conv(s.Args[0]), conv(s.Args[1]), conv(s.Args[2])
			out = append(out, displaylist.Segment{Kind: displaylist.SegmentCubic, P0: cur, P1: c1, P2: c2, P3: next})
			cur = next
		}
	}
	closeContour()
	return out
}

type fontChain []FontProvider

// ChainFonts returns a provider that tries providers in order until one
// produces an outline. Provider errors stop the chain. Nil providers are
// ignored.
func ChainFonts(providers ...FontProvider) FontProvider {
	out := make(fontChain, 0, len(providers))
	for _, p := range providers {
		if p != nil {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (c fontChain) GlyphOutline(info *displaylist.FontInfo, glyph displaylist.Glyph) ([]displaylist.Segment, bool, error) {
	for _, p := range c {
		seg, ok, err := p.GlyphOutline(info, glyph)
		if err != nil {
			return nil, false, err
		}
		if ok {
			return seg, true, nil
		}
	}
	return nil, false, nil
}

func defaultTextFonts() FontProvider {
	defaultFontsOnce.Do(func() {
		if sys, err := SystemFonts(); err == nil {
			defaultFonts = sys
		}
	})
	return ChainFonts(defaultFonts, BuiltinFonts())
}
