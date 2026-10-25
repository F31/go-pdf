// Package displaylist defines the stable contract between go-pdf and its
// consumers. G0 froze the type skeleton at Version 1.0; field additions since
// then increment the minor version (currently 2.0).
package displaylist

import (
	"context"
	"image"
	"io"
	"sync"
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
	Profile []byte `json:",omitempty"`
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
	Shading    *Shading
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

// CCITTParams carries CCITTFaxDecode parameters that are needed by external
// G3/G4 providers. It is an add-only display-list field.
type CCITTParams struct {
	K                int
	Columns          int
	Rows             int
	BlackIsZero      bool
	EncodedByteAlign bool
	EndOfLine        bool
	EndOfBlock       bool
	EndOfData        bool
}

type ImageInfo struct {
	Width, Height    int
	BitsPerComponent int
	ColorSpace       string
	Encoding         ImageEncoding
	Decode           []float64
	Interpolate      bool
	Inline           bool
	CCITT            *CCITTParams
}

type ImageRef interface {
	Raw() ([]byte, error)
	Decode(ctx context.Context) (image.Image, error)
}

// ImageSamples preserves normalized source-color samples before any output
// color conversion. Data is interleaved and row-major, top-down. Components is
// the number of source components per pixel. ColorSpace may be "ICCBased"; in
// that case Profile contains the embedded ICC profile.
//
// Callers must call Close when the samples are no longer needed. ImageRefs
// that cannot expose source samples do not implement ImageSampleRef.
type ImageSamples struct {
	Width, Height int
	Components    int
	ColorSpace    string
	Profile       []byte
	Data          []float32

	release func()
	once    sync.Once
}

// NewImageSamples builds a sample buffer owned by release. release may be nil.
func NewImageSamples(width, height, components int, space string, profile []byte, data []float32, release func()) *ImageSamples {
	return &ImageSamples{
		Width: width, Height: height, Components: components,
		ColorSpace: space, Profile: profile, Data: data, release: release,
	}
}

// Close releases the sample buffer lease. It is idempotent.
func (s *ImageSamples) Close() {
	if s == nil {
		return
	}
	s.once.Do(func() {
		if s.release != nil {
			s.release()
		}
	})
}

// ImageSampleRef is implemented by image resources that can expose normalized
// source samples without first converting them to sRGB.
type ImageSampleRef interface {
	DecodeSamples(ctx context.Context) (*ImageSamples, error)
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

// FontSource exposes the decoded embedded font program for providers that need
// to load their own font engine. It is not serialized in display-list JSON.
type FontSource interface {
	io.ReaderAt
	Size() int64
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
	Source      FontSource  `json:"-"`
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
	Subtype     string
	BBox        Rect
	Items       []Item
	Clips       []Clip
	Backdrop    [4]float64
	HasBackdrop bool
}

// PaintState preserves separate fill/stroke alpha when a single effective
// Alpha cannot represent both. Consumers that only understand Alpha may use it
// as the conservative composite value.
type PaintState struct {
	FillAlpha   float64
	StrokeAlpha float64
}

// TextSpan maps a Unicode substitution (usually ActualText) to a glyph range.
// GlyphEnd is inclusive. Text must not be duplicated into every glyph.
type TextSpan struct {
	Text       string
	GlyphStart int
	GlyphEnd   int
	ActualText bool
	MCID       int
	StructRef  int
}

type Common struct {
	Z          int
	Clip       int
	Alpha      float64
	Blend      string
	Layer      string
	MCID       int
	StructRef  int
	SoftMask   *SoftMask
	PaintState *PaintState `json:",omitempty"`
	Flags      Flags
	Bounds     Rect
	ID         int
	Source     SrcRef
	Sig        uint64
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
	Line       LineStyle
	TextFlags  TextFlags
	Spans      []TextSpan `json:",omitempty"`
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

// MeshColorFunc maps mesh colour components through the shading function.
type MeshColorFunc func([]float64) ([]float64, error)

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
	// MeshComponents is the number of decoded colour components per vertex.
	// Zero means the shading colour space's component count.
	MeshComponents int `json:"-"`
	// MeshColor applies the Type 4-7 /Function. It is not serialized.
	MeshColor MeshColorFunc `json:"-"`
	Matrix    Matrix
	Grid      *ShadingGrid
}

type Backdrop struct {
	Color  [4]float64
	Bounds Rect
}

type Group struct {
	Common
	Name            string
	Items           []Item
	Clips           []Clip
	Isolated        bool
	Knockout        bool
	ColorSpace      string
	BlendColorSpace string
	Backdrop        *Backdrop
}

type Clip struct {
	Path      []Segment
	Rule      FillRule
	Bounds    Rect
	Intersect []Clip `json:",omitempty"`
}

type Link struct {
	Rect Rect
	URI  string
	Page int
}

// Annotation is a generic page annotation or form widget.
type Annotation struct {
	Subtype    string
	Rect       Rect
	Contents   string
	URI        string
	Page       int
	Flags      int
	FieldName  string
	FieldValue string
	FieldType  string
	Appearance *AnnotationAppearance
}

// AnnotationAppearance contains one appearance stream expanded into page-space
// display items and clips.
type AnnotationAppearance struct {
	Items []Item
	Clips []Clip
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
	Index       int
	Width       float64
	Height      float64
	Rotate      int
	Label       string
	MediaBox    Rect
	CropBox     Rect
	Clips       []Clip
	Items       []Item
	Diag        []Diagnostic
	Links       []Link
	Annotations []Annotation
	Background  *BackgroundHint
	Scanned     bool
}
