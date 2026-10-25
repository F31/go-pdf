package render

import (
	"context"
	"errors"

	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/extension"
)

// extensionFontProvider adapts an extension.FontEngine to the render
// FontProvider surface. It is scoped to one render call and closes every loaded
// face when the call finishes.
type extensionFontProvider struct {
	ctx    context.Context
	engine extension.FontEngine
	faces  map[*displaylist.FontInfo]extension.FontFace
	failed map[*displaylist.FontInfo]error
}

func newExtensionFontProvider(ctx context.Context, engine extension.FontEngine) *extensionFontProvider {
	return &extensionFontProvider{
		ctx: ctx, engine: engine,
		faces:  map[*displaylist.FontInfo]extension.FontFace{},
		failed: map[*displaylist.FontInfo]error{},
	}
}

func (p *extensionFontProvider) GlyphOutline(info *displaylist.FontInfo, glyph displaylist.Glyph) ([]displaylist.Segment, bool, error) {
	if p == nil || p.engine == nil || info == nil || info.Source == nil {
		return nil, false, nil
	}
	face, err := p.face(info)
	if err != nil {
		return nil, false, err
	}
	g, ok, err := face.Glyph(p.ctx, glyph.Code, glyph.CID)
	if err != nil || !ok {
		return nil, ok, err
	}
	out := make([]displaylist.Segment, 0, len(g.Outline))
	for _, s := range g.Outline {
		kind, ok := displaylistOp(s.Op)
		if !ok {
			return nil, false, errors.New("render: unsupported extension outline operation")
		}
		out = append(out, displaylist.Segment{
			Kind: kind,
			P0:   displaylist.Point{X: s.P0.X, Y: s.P0.Y},
			P1:   displaylist.Point{X: s.P1.X, Y: s.P1.Y},
			P2:   displaylist.Point{X: s.P2.X, Y: s.P2.Y},
			P3:   displaylist.Point{X: s.P3.X, Y: s.P3.Y},
		})
	}
	return out, len(out) > 0, nil
}

func (p *extensionFontProvider) face(info *displaylist.FontInfo) (extension.FontFace, error) {
	if face, ok := p.faces[info]; ok {
		return face, nil
	}
	if err, ok := p.failed[info]; ok {
		return nil, err
	}
	spec := extension.FontSpec{
		BaseFont: info.BaseFont,
		Subtype:  info.Kind,
		Embedded: info.Embedded,
		CID:      info.Kind == "Type0",
	}
	face, err := p.engine.Load(p.ctx, info.Source, info.Source.Size(), spec)
	if err != nil {
		p.failed[info] = err
		return nil, err
	}
	p.faces[info] = face
	return face, nil
}

func (p *extensionFontProvider) Close() error {
	if p == nil {
		return nil
	}
	var first error
	for _, face := range p.faces {
		if err := face.Close(); err != nil && first == nil {
			first = err
		}
	}
	p.faces = nil
	return first
}

func displaylistOp(op extension.OutlineOp) (displaylist.SegmentKind, bool) {
	switch op {
	case extension.OutlineMove:
		return displaylist.SegmentMove, true
	case extension.OutlineLine:
		return displaylist.SegmentLine, true
	case extension.OutlineCubic:
		return displaylist.SegmentCubic, true
	case extension.OutlineClose:
		return displaylist.SegmentClose, true
	default:
		return 0, false
	}
}
