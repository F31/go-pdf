package extension

import (
	"context"
	"io"
)

// FontSpec describes an embedded or substituted font program.
type FontSpec struct {
	BaseFont string
	Subtype  string
	Embedded bool
	CID      bool
	Vertical bool
	Matrix   [6]float64
}

// FontMetrics uses 1/1000 text-space units unless documented otherwise.
type FontMetrics struct {
	UnitsPerEm  int
	Ascent      float64
	Descent     float64
	CapHeight   float64
	XHeight     float64
	ItalicAngle float64
	AvgWidth    float64
}

// OutlineOp is one outline operation. Move and Line use P0. Cubic uses P0, P1,
// P2 and P3. Close uses no points.
type OutlineOp uint8

const (
	OutlineMove OutlineOp = iota
	OutlineLine
	OutlineCubic
	OutlineClose
)

type Point struct {
	X float64
	Y float64
}

type OutlineSegment struct {
	Op             OutlineOp
	P0, P1, P2, P3 Point
}

// Glyph is a resolved glyph. Outline is in 1/1000 text space and Y-up, matching
// displaylist.FontInfo.GlyphSource. Advance is in 1/1000 text space.
type Glyph struct {
	GID     uint32
	Code    uint32
	CID     uint32
	Advance float64
	Outline []OutlineSegment
}

// FontFace is one loaded face. It is not safe for concurrent use unless its
// provider explicitly documents otherwise.
type FontFace interface {
	Metrics() FontMetrics
	Glyph(ctx context.Context, code, cid uint32) (Glyph, bool, error)
	Close() error
}

// FontEngine loads font programs and resolves glyphs. It does not perform
// shaping or fallback selection; those remain separate optional capabilities.
type FontEngine interface {
	Provider
	Load(ctx context.Context, src io.ReaderAt, size int64, spec FontSpec) (FontFace, error)
}
