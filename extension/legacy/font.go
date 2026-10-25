package legacy

import (
	"context"
	"errors"
	"io"

	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/extension"
	"github.com/F31/go-pdf/render"
)

// NewFontEngine adapts a legacy render.FontProvider to an outline-only font
// engine. Metrics, advances and GIDs are not recovered by this bridge.
func NewFontEngine(info extension.ProviderInfo, p render.FontProvider) extension.FontEngine {
	if p == nil {
		return nil
	}
	return fontEngine{info: info, legacy: p}
}

type fontEngine struct {
	info   extension.ProviderInfo
	legacy render.FontProvider
}

func (e fontEngine) Info() extension.ProviderInfo  { return e.info }
func (e fontEngine) Features() []extension.Feature { return nil }

func (e fontEngine) Load(_ context.Context, _ io.ReaderAt, _ int64, spec extension.FontSpec) (extension.FontFace, error) {
	return &fontFace{
		provider: e.legacy,
		info: displaylist.FontInfo{
			BaseFont: spec.BaseFont,
			Kind:     spec.Subtype,
			Embedded: spec.Embedded,
		},
	}, nil
}

type fontFace struct {
	provider render.FontProvider
	info     displaylist.FontInfo
}

func (f *fontFace) Metrics() extension.FontMetrics { return extension.FontMetrics{} }

func (f *fontFace) Glyph(ctx context.Context, code, cid uint32) (extension.Glyph, bool, error) {
	if err := ctx.Err(); err != nil {
		return extension.Glyph{}, false, err
	}
	segments, ok, err := f.provider.GlyphOutline(&f.info, displaylist.Glyph{Code: code, CID: cid})
	if err != nil || !ok {
		return extension.Glyph{}, ok, err
	}
	out := make([]extension.OutlineSegment, 0, len(segments))
	for _, s := range segments {
		op, ok := extensionOutlineOp(s.Kind)
		if !ok {
			return extension.Glyph{}, false, errors.New("legacy: unsupported outline operation")
		}
		out = append(out, extension.OutlineSegment{
			Op: op,
			P0: extension.Point{X: s.P0.X, Y: s.P0.Y},
			P1: extension.Point{X: s.P1.X, Y: s.P1.Y},
			P2: extension.Point{X: s.P2.X, Y: s.P2.Y},
			P3: extension.Point{X: s.P3.X, Y: s.P3.Y},
		})
	}
	return extension.Glyph{Code: code, CID: cid, Outline: out}, true, nil
}

func (*fontFace) Close() error { return nil }

func extensionOutlineOp(kind displaylist.SegmentKind) (extension.OutlineOp, bool) {
	switch kind {
	case displaylist.SegmentMove:
		return extension.OutlineMove, true
	case displaylist.SegmentLine:
		return extension.OutlineLine, true
	case displaylist.SegmentCubic:
		return extension.OutlineCubic, true
	case displaylist.SegmentClose:
		return extension.OutlineClose, true
	default:
		return 0, false
	}
}
