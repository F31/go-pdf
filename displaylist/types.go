// Package displaylist defines the stable contract between go-pdf and its
// consumers. G0 froze the type skeleton at Version 1.0; field additions since
// then increment the minor version (currently 1.11).
package displaylist

import (
	"context"
	"image"
)

// Matrix is [a b c d e f] in page space: x' = a*x + c*y + e, y' = b*x + d*y + f.
type Matrix [6]float64

type Point struct{ X, Y float64 }

type Rect struct{ X0, Y0, X1, Y1 float64 }

type Flags uint32

const (
	FlagOutOfBounds Flags = 1 << iota
	FlagApprox
	FlagFromType3
	FlagInline
	FlagClipped
)

type TextFlags uint32

const (
	TextFakeBold TextFlags = 1 << iota
	TextStroked
	TextInvisible
	TextClipped
)

type PaintKind int

const (
	PaintSolid PaintKind = iota
	PaintShading
	PaintPattern
)

type Paint struct {
	Kind    PaintKind
	RGBA    [4]float64
	Space   string
	Raw     []float64
	Pattern *Pattern
	Approx  bool
}

// Pattern records a tiling or shading pattern. Content is the decoded pattern
// content stream for tiling patterns; Matrix maps pattern space to page space.
type Pattern struct {
	Name       string
	Type       int
	PaintType  int
	TilingType int
	BBox       Rect
	XStep      float64
	YStep      float64
	Matrix     Matrix
	Base       []float64
	Content    []byte
	Items      []Item
	Clips      []Clip
}

type FillRule int

const (
	NonZero FillRule = iota
	EvenOdd
)

type SegmentKind int

const (
	SegmentMove SegmentKind = iota
	SegmentLine
	SegmentCubic
	SegmentClose
)

type Segment struct {
	Kind           SegmentKind
	P0, P1, P2, P3 Point
}

type LineStyle struct {
	Width float64
	Cap   int
	Join  int
	Miter float64
	Dash  []float64
	Phase float64
}

type PathHint int

const (
	PathGeneric PathHint = iota
	PathRect
	PathRoundRect
	PathEllipse
	PathLine
	PathPolyline
)

type ImageRole int

const (
	ImagePhoto ImageRole = iota
	ImageMaskRole
	ImageBackground
	ImageTile
)

type ImageEncoding string

type ImageInfo struct {
	Width, Height    int
	BitsPerComponent int
	ColorSpace       string
	Encoding         ImageEncoding
	Decode           []float64
	Interpolate      bool
	Inline           bool
}

type ImageRef interface {
	Raw() ([]byte, error)
	Decode(ctx context.Context) (image.Image, error)
}

type MaskRef struct {
	IsColorKey bool
	ColorKey   []float64
	Image      ImageRef
}

type ShadingKind int

const (
	ShadingAxial ShadingKind = iota
	ShadingRadial
	ShadingFunction
	ShadingMesh
)

type ColorStop struct {
	Offset float64
	Color  [4]float64
}

type ShadingGeometry struct {
	X0, Y0, X1, Y1, R0, R1 float64
}

type SrcRef struct {
	OpIndex int
	Obj     int
}

type FontMetrics struct {
	UnitsPerEm  int
	Ascent      int
	Descent     int
	CapHeight   int
	XHeight     int
	AvgWidth    int
	Advances    map[uint32]int
	Substituted bool
}

type GlyphSource interface {
	GlyphOutline(code, cid uint32) ([]Segment, bool)
}

type FontInfo struct {
	BaseFont    string
	Family      string
	Bold        bool
	Italic      bool
	Serif       bool
	Mono        bool
	Embedded    bool
	Subset      bool
	Kind        string
	Ascent      float64
	Descent     float64
	ItalicAngle float64
	Metrics     FontMetrics
	GlyphSource GlyphSource `json:"-"`
}

type Glyph struct {
	Text      string
	UnicodeOK bool
	Code      uint32
	CID       uint32
	Origin    Point
	Advance   float64
	Matrix    Matrix
}

type SoftMask struct {
	Subtype string
	BBox    Rect
	Items   []Item
	Clips   []Clip
}

type Common struct {
	Z         int
	Clip      int
	Alpha     float64
	Blend     string
	Layer     string
	MCID      int
	StructRef int
	SoftMask  *SoftMask
	Flags     Flags
	Bounds    Rect
	ID        int
	Source    SrcRef
	Sig       uint64
}

func (c *Common) Base() *Common { return c }

type Item interface{ Base() *Common }

type TextRun struct {
	Common
	Font       *FontInfo
	Size       float64
	ActualText string
	Fill       Paint
	Stroke     *Paint
	RenderMode int
	Vertical   bool
	Glyphs     []Glyph
	CharSpace  float64
	WordSpace  float64
	HScale     float64
	Rise       float64
	TextFlags  TextFlags
}

type Path struct {
	Common
	Segments []Segment
	Fill     *Paint
	FillRule FillRule
	Stroke   *Paint
	Line     LineStyle
	Hint     PathHint
	Closed   bool
}

type Image struct {
	Common
	Matrix      Matrix
	Info        ImageInfo
	Data        ImageRef
	Mask        *MaskRef
	Fill        *Paint
	Interpolate bool
	Decode      []float64
	Role        ImageRole
}

type ShadingGrid struct {
	Width  int
	Height int
	Domain [4]float64
	Colors [][4]float64
}

type Shading struct {
	Common
	Kind              ShadingKind
	Geometry          any
	Stops             []ColorStop
	Extend            [2]bool
	MaxDeltaE         float64
	Raw               []byte
	BitsPerCoordinate int
	BitsPerComponent  int
	BitsPerFlag       int
	Decode            []float64
	ColorSpace        string
	MeshType          int
	VerticesPerRow    int
	Matrix            Matrix
	Grid              *ShadingGrid
}

type Group struct {
	Common
	Name       string
	Items      []Item
	Clips      []Clip
	Isolated   bool
	Knockout   bool
	ColorSpace string
}

type Clip struct {
	Path   []Segment
	Rule   FillRule
	Bounds Rect
}

type Link struct {
	Rect Rect
	URI  string
	Page int
}

type BackgroundHint struct {
	IsImage bool
	Color   [4]float64
	Image   *Image
}

type Diagnostic struct {
	Level  string
	Code   string
	Msg    string
	Page   int
	Obj    int
	Offset int64
}

type Page struct {
	Index      int
	Width      float64
	Height     float64
	Rotate     int
	MediaBox   Rect
	CropBox    Rect
	Clips      []Clip
	Items      []Item
	Diag       []Diagnostic
	Links      []Link
	Background *BackgroundHint
	Scanned    bool
}
