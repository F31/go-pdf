package displaylist

import (
	"encoding/binary"
	"hash/fnv"
	"math"
)

// Signature returns a stable content signature for cross-page duplicate
// detection. Geometry is measured relative to the item's Bounds origin so the
// same visual element at different positions has the same signature.
func Signature(it Item) uint64 {
	w := &sigWriter{h: fnv.New64a()}
	c := it.Base()
	w.f(c.Alpha)
	w.s(c.Blend)
	ox, oy := c.Bounds.X0, c.Bounds.Y0
	switch v := it.(type) {
	case *TextRun:
		w.s("text")
		if v.Font != nil {
			w.s(v.Font.BaseFont)
		}
		w.f(v.Size)
		w.s(v.ActualText)
		w.paint(v.Fill)
		if v.Stroke != nil {
			w.paint(*v.Stroke)
		} else {
			w.s("no-stroke")
		}
		w.i(v.RenderMode)
		w.b(v.Vertical)
		w.f(v.CharSpace)
		w.f(v.WordSpace)
		w.f(v.HScale)
		w.f(v.Rise)
		w.i(int(v.TextFlags))
		for _, g := range v.Glyphs {
			w.s(g.Text)
			w.b(g.UnicodeOK)
			w.f(g.Origin.X - ox)
			w.f(g.Origin.Y - oy)
			w.f(g.Advance)
		}
	case *Path:
		w.s("path")
		if v.Fill != nil {
			w.paint(*v.Fill)
		} else {
			w.s("no-fill")
		}
		if v.Stroke != nil {
			w.paint(*v.Stroke)
		} else {
			w.s("no-stroke")
		}
		w.i(int(v.FillRule))
		w.i(int(v.Hint))
		w.b(v.Closed)
		w.f(v.Line.Width)
		w.i(v.Line.Cap)
		w.i(v.Line.Join)
		w.f(v.Line.Miter)
		for _, d := range v.Line.Dash {
			w.f(d)
		}
		w.f(v.Line.Phase)
		for _, s := range v.Segments {
			w.i(int(s.Kind))
			switch s.Kind {
			case SegmentMove:
				w.pt(s.P0, ox, oy)
			case SegmentLine, SegmentClose:
				w.pt(s.P0, ox, oy)
				w.pt(s.P3, ox, oy)
			case SegmentCubic:
				w.pt(s.P0, ox, oy)
				w.pt(s.P1, ox, oy)
				w.pt(s.P2, ox, oy)
				w.pt(s.P3, ox, oy)
			}
		}
	case *Image:
		w.s("image")
		w.f(v.Matrix[0])
		w.f(v.Matrix[1])
		w.f(v.Matrix[2])
		w.f(v.Matrix[3])
		w.f(v.Matrix[4] - ox)
		w.f(v.Matrix[5] - oy)
		w.i(v.Info.Width)
		w.i(v.Info.Height)
		w.i(v.Info.BitsPerComponent)
		w.s(v.Info.ColorSpace)
		w.s(string(v.Info.Encoding))
		for _, d := range v.Info.Decode {
			w.f(d)
		}
		w.b(v.Info.Interpolate)
		w.b(v.Info.Inline)
		w.i(int(v.Role))
		if v.Fill != nil {
			w.paint(*v.Fill)
		}
		if v.Mask != nil {
			w.b(v.Mask.IsColorKey)
		}
	case *Shading:
		w.s("shading")
		w.i(int(v.Kind))
		switch g := v.Geometry.(type) {
		case ShadingGeometry:
			w.f(g.X0 - ox)
			w.f(g.Y0 - oy)
			w.f(g.X1 - ox)
			w.f(g.Y1 - oy)
			w.f(g.R0)
			w.f(g.R1)
		}
		for _, s := range v.Stops {
			w.f(s.Offset)
			w.pt(Point{X: s.Color[0], Y: s.Color[1]}, 0, 0)
			w.pt(Point{X: s.Color[2], Y: s.Color[3]}, 0, 0)
		}
		w.b(v.Extend[0])
		w.b(v.Extend[1])
		w.u(uint64(len(v.Raw)))
		w.h.Write(v.Raw)
		w.i(v.BitsPerCoordinate)
		w.i(v.BitsPerComponent)
		w.i(v.BitsPerFlag)
		w.s(v.ColorSpace)
		for _, d := range v.Decode {
			w.f(d)
		}
		w.i(v.MeshType)
		w.i(v.VerticesPerRow)
		for _, f := range v.Matrix {
			w.f(f)
		}
		if v.Grid != nil {
			w.i(v.Grid.Width)
			w.i(v.Grid.Height)
			for _, f := range v.Grid.Domain {
				w.f(f)
			}
			for _, c := range v.Grid.Colors {
				for _, f := range c {
					w.f(f)
				}
			}
		}
	case *Group:
		w.s("group")
		w.s(v.Name)
		for _, child := range v.Items {
			w.u(Signature(child))
		}
	default:
		w.s("item")
	}
	return w.h.Sum64()
}

type sigWriter struct {
	h interface {
		Write([]byte) (int, error)
		Sum64() uint64
	}
}

func (w *sigWriter) b(v bool) {
	if v {
		w.h.Write([]byte{1})
	} else {
		w.h.Write([]byte{0})
	}
}

func (w *sigWriter) i(v int) { w.u(uint64(v)) }
func (w *sigWriter) u(v uint64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], v)
	w.h.Write(b[:])
}

func (w *sigWriter) f(v float64) {
	if v == 0 {
		v = 0
	}
	w.u(math.Float64bits(v))
}

func (w *sigWriter) s(v string) {
	w.u(uint64(len(v)))
	w.h.Write([]byte(v))
}

func (w *sigWriter) pt(p Point, ox, oy float64) {
	w.f(p.X - ox)
	w.f(p.Y - oy)
}

func (w *sigWriter) paint(p Paint) {
	w.i(int(p.Kind))
	for _, v := range p.RGBA {
		w.f(v)
	}
	w.s(p.Space)
	for _, v := range p.Raw {
		w.f(v)
	}
	if p.Pattern != nil {
		w.s(p.Pattern.Name)
		w.i(p.Pattern.Type)
		w.i(p.Pattern.PaintType)
		w.i(p.Pattern.TilingType)
		w.f(p.Pattern.BBox.X0)
		w.f(p.Pattern.BBox.Y0)
		w.f(p.Pattern.BBox.X1)
		w.f(p.Pattern.BBox.Y1)
		w.f(p.Pattern.XStep)
		w.f(p.Pattern.YStep)
		for _, v := range p.Pattern.Matrix {
			w.f(v)
		}
		for _, v := range p.Pattern.Base {
			w.f(v)
		}
		w.u(uint64(len(p.Pattern.Content)))
		w.h.Write(p.Pattern.Content)
	}
	w.b(p.Approx)
}
