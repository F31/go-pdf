package font

import (
	"math"

	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/F31/go-pdf/displaylist"
)

// GlyphOutline returns the glyph outline in 1/1000 text-space units with the
// y axis pointing up, matching the coordinate system used by
// displaylist.Glyph.Matrix. The boolean is false when the font has no outline
// program or the code cannot be mapped to a glyph.
func (f *Font) GlyphOutline(c Code) ([]displaylist.Segment, bool) {
	gid, ok := f.outlineGID(c)
	if !ok {
		return nil, false
	}
	var buf sfnt.Buffer
	segs, err := f.program.face.LoadGlyph(&buf, gid, fixed.I(f.program.upem), nil)
	if err != nil {
		return nil, false
	}
	return outlineSegments(segs, f.program.upem), true
}

func (f *Font) outlineGID(c Code) (sfnt.GlyphIndex, bool) {
	if f == nil || f.program == nil || f.type3 {
		return 0, false
	}
	if f.isCID && (f.cidToGIDIdentity || len(f.cidToGID) > 0) {
		if gid, ok := f.gidForCID(c.CID); ok {
			return gid, gid != 0
		}
	}
	text, ok := f.DecodeCode(c)
	if !ok {
		return 0, false
	}
	r := firstRune(text)
	if r == 0 {
		return 0, false
	}
	var buf sfnt.Buffer
	gid, err := f.program.face.GlyphIndex(&buf, r)
	if err != nil || gid == 0 {
		return 0, false
	}
	return gid, true
}

func outlineSegments(in sfnt.Segments, upem int) []displaylist.Segment {
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

func outlineBounds(seg []displaylist.Segment) displaylist.Rect {
	if len(seg) == 0 {
		return displaylist.Rect{}
	}
	r := displaylist.Rect{X0: math.Inf(1), Y0: math.Inf(1), X1: math.Inf(-1), Y1: math.Inf(-1)}
	add := func(p displaylist.Point) {
		r.X0 = math.Min(r.X0, p.X)
		r.Y0 = math.Min(r.Y0, p.Y)
		r.X1 = math.Max(r.X1, p.X)
		r.Y1 = math.Max(r.Y1, p.Y)
	}
	for _, s := range seg {
		switch s.Kind {
		case displaylist.SegmentMove:
			add(s.P0)
		case displaylist.SegmentLine, displaylist.SegmentClose:
			add(s.P0)
			add(s.P3)
		case displaylist.SegmentCubic:
			add(s.P0)
			add(s.P1)
			add(s.P2)
			add(s.P3)
		}
	}
	return r
}
