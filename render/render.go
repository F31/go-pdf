// Package render provides approximate region rasterization for display lists.
package render

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"math"
	"sort"
	"strings"

	"golang.org/x/image/vector"

	"github.com/F31/go-pdf"
	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/extension"
	pdfcolor "github.com/F31/go-pdf/internal/color"
	"github.com/F31/go-pdf/internal/errs"
	"github.com/F31/go-pdf/internal/syntax"
)

const (
	defaultDPI = 220
	maxDPI     = 300
)

// AnnotationMode selects annotation appearance visibility.
type AnnotationMode uint8

const (
	AnnotationNone AnnotationMode = iota
	AnnotationView
)

// PageOptions controls a RenderRegion call.
type PageOptions struct {
	DPI        int
	Background string // "white" or "transparent" (default)
	// IncludeText renders text from embedded sfnt outlines. When no Fonts or
	// FontEngine is configured it uses installed system fonts and then the
	// built-in Go fallback. It defaults to false; callers that need text must
	// set it explicitly. Set Fonts explicitly for host-independent output.
	IncludeText bool
	// Annotations overlays available annotation appearance streams.
	Annotations AnnotationMode
	Exclude     func(displaylist.Item) bool
	MaxPixels   int
	// Fonts supplies outlines for text runs without an embedded GlyphSource,
	// such as base-14 fonts. It takes precedence over FontEngine.
	Fonts FontProvider
	// FontEngine supplies outlines from a provider that reads the embedded
	// FontInfo.Source. It is used only when Fonts is nil.
	FontEngine extension.FontEngine
	// Memory optionally accounts transient raster buffers. Pass the Page itself
	// to charge the owning document's live-memory budget. The returned image is
	// owned by the caller and is not covered by this lease.
	Memory MemoryBudget
}

// MemoryBudget accounts transient raster allocations such as path/text
// buffers, transparency groups and soft masks.
type MemoryBudget interface {
	AcquireMemory(bytes int64) (func(), error)
}

// FontProvider supplies glyph outlines when a text run has no embedded
// displaylist.GlyphSource. Outlines use the same 1/1000 text-space units as
// FontInfo.GlyphSource and are scaled by Glyph.Matrix by the renderer.
type FontProvider interface {
	GlyphOutline(info *displaylist.FontInfo, glyph displaylist.Glyph) ([]displaylist.Segment, bool, error)
}

// RenderRegion rasterizes the intersection of box with a page. Coordinates are
// in page space (points, top-left origin). Text uses embedded sfnt outlines;
// items without outlines are skipped. The returned image is not covered by the
// page memory budget; use RenderRegionLease for an owned raster.
func RenderRegion(ctx context.Context, page *pdf.Page, box displaylist.Rect, opts PageOptions) (image.Image, error) {
	if page == nil {
		return nil, errs.Wrap(errs.ErrClosed, "RND-PAGE", "render region", -1, 0, -1)
	}
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap(errs.ErrCanceled, "RND-CANCELED", "render region", -1, 0, -1)
	}
	w, h, scale, err := regionSpec(box, opts)
	if err != nil {
		return nil, err
	}
	dl, err := page.DisplayList(ctx)
	if err != nil {
		return nil, err
	}
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	if err := renderRegionInto(ctx, page, box, opts, out, scale, dl); err != nil {
		return nil, err
	}
	return out, nil
}

// RenderRegionLease is RenderRegion with the output RGBA buffer charged to the
// page's live-memory budget. Callers must Release the returned raster.
func RenderRegionLease(ctx context.Context, page *pdf.Page, box displaylist.Rect, opts PageOptions) (*Raster, error) {
	if page == nil {
		return nil, errs.Wrap(errs.ErrClosed, "RND-PAGE", "render region", -1, 0, -1)
	}
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap(errs.ErrCanceled, "RND-CANCELED", "render region", -1, 0, -1)
	}
	w, h, scale, err := regionSpec(box, opts)
	if err != nil {
		return nil, err
	}
	dl, err := page.DisplayList(ctx)
	if err != nil {
		return nil, err
	}
	memory := memoryFor(page, opts)
	release, err := memory.AcquireMemory(int64(w) * int64(h) * 4)
	if err != nil {
		return nil, err
	}
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	if err := renderRegionInto(ctx, page, box, opts, out, scale, dl); err != nil {
		release()
		return nil, err
	}
	return &Raster{Image: out, release: release}, nil
}

func regionSpec(box displaylist.Rect, opts PageOptions) (int, int, float64, error) {
	if box.X1 <= box.X0 || box.Y1 <= box.Y0 {
		return 0, 0, 0, errs.Wrap(errs.ErrCorrupt, "RND-BOX", "render region", -1, 0, -1)
	}
	dpi := opts.DPI
	if dpi <= 0 {
		dpi = defaultDPI
	}
	if dpi > maxDPI {
		dpi = maxDPI
	}
	scale := float64(dpi) / 72
	w := int(math.Ceil((box.X1 - box.X0) * scale))
	h := int(math.Ceil((box.Y1 - box.Y0) * scale))
	if w <= 0 || h <= 0 {
		return 0, 0, 0, errs.Wrap(errs.ErrCorrupt, "RND-BOX", "render region", -1, 0, -1)
	}
	maxPixels := opts.MaxPixels
	if maxPixels <= 0 {
		maxPixels = 64 << 20
	}
	if int64(w)*int64(h) > int64(maxPixels) {
		return 0, 0, 0, errs.Wrap(errs.ErrBudget, "BUD-RASTER-PIXELS", "render region", -1, 0, -1)
	}
	return w, h, scale, nil
}

func memoryFor(page *pdf.Page, opts PageOptions) MemoryBudget {
	if opts.Memory != nil {
		return opts.Memory
	}
	if m, ok := any(page).(MemoryBudget); ok {
		return m
	}
	return noMemory{}
}

type noMemory struct{}

func (noMemory) AcquireMemory(int64) (func(), error) { return func() {}, nil }

func renderRegionLeaseDL(ctx context.Context, page *pdf.Page, box displaylist.Rect, opts PageOptions, dl *displaylist.Page) (*Raster, error) {
	w, h, scale, err := regionSpec(box, opts)
	if err != nil {
		return nil, err
	}
	memory := memoryFor(page, opts)
	release, err := memory.AcquireMemory(int64(w) * int64(h) * 4)
	if err != nil {
		return nil, err
	}
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	if err := renderRegionInto(ctx, page, box, opts, out, scale, dl); err != nil {
		release()
		return nil, err
	}
	return &Raster{Image: out, release: release}, nil
}

func renderRegionInto(ctx context.Context, page *pdf.Page, box displaylist.Rect, opts PageOptions, out *image.RGBA, scale float64, dl *displaylist.Page) error {
	if opts.Fonts == nil && opts.FontEngine != nil {
		provider := newExtensionFontProvider(ctx, opts.FontEngine)
		defer provider.Close()
		opts.Fonts = provider
	}
	if opts.Fonts == nil && opts.IncludeText {
		opts.Fonts = defaultTextFonts()
	}
	if opts.Background == "white" {
		draw.Draw(out, out.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	}
	rr := &regionRenderer{
		ctx: ctx, dl: dl, out: out, box: box, scale: scale,
		pageToPx: displaylist.Matrix{scale, 0, 0, scale, -box.X0 * scale, -box.Y0 * scale},
		opts:     opts, clips: map[int]*image.Alpha{}, clipList: dl.Clips,
		softMasks: map[*displaylist.SoftMask]*image.Alpha{}, memory: memoryFor(page, opts),
	}
	defer rr.releaseAll()
	for _, it := range dl.Items {
		if opts.Exclude != nil && opts.Exclude(it) {
			continue
		}
		if err := rr.renderItem(it); err != nil {
			return err
		}
	}
	if opts.Annotations != AnnotationNone {
		for _, a := range dl.Annotations {
			if a.Appearance == nil || len(a.Appearance.Items) == 0 || !annotationVisible(a, opts.Annotations) {
				continue
			}
			child := &regionRenderer{
				ctx: rr.ctx, dl: rr.dl, out: rr.out, box: rr.box, scale: rr.scale,
				pageToPx: rr.pageToPx, opts: rr.opts, clips: map[int]*image.Alpha{},
				clipList: a.Appearance.Clips, softMasks: map[*displaylist.SoftMask]*image.Alpha{},
				memory: rr.memory,
			}
			for _, it := range a.Appearance.Items {
				if opts.Exclude != nil && opts.Exclude(it) {
					continue
				}
				if err := child.renderItem(it); err != nil {
					child.releaseAll()
					return err
				}
			}
			child.releaseAll()
		}
	}
	return nil
}

func (rr *regionRenderer) renderItem(it displaylist.Item) error {
	c := it.Base()
	saved := rr.softMask
	if !rr.inMask && c.SoftMask != nil {
		m, err := rr.softMaskFor(c.SoftMask)
		if err != nil {
			return err
		}
		rr.softMask = m
	}
	err := rr.dispatchItem(it)
	rr.softMask = saved
	if err == nil && rr.memErr != nil {
		err = rr.memErr
	}
	return err
}

func (rr *regionRenderer) dispatchItem(it displaylist.Item) error {
	switch v := it.(type) {
	case *displaylist.Path:
		return rr.path(v)
	case *displaylist.Image:
		return rr.image(v)
	case *displaylist.Shading:
		return rr.shading(v)
	case *displaylist.Group:
		return rr.group(v)
	case *displaylist.TextRun:
		if rr.opts.IncludeText {
			return rr.text(v)
		}
	}
	return nil
}

func (rr *regionRenderer) group(g *displaylist.Group) error {
	if g == nil || len(g.Items) == 0 {
		return nil
	}
	// Knockout groups composite each element against the group backdrop instead
	// of blending elements with each other.
	if g.Knockout {
		off := image.NewRGBA(rr.out.Bounds())
		release, err := rr.acquire(int64(len(off.Pix)))
		if err != nil {
			return err
		}
		defer release()
		var base *image.RGBA
		if !g.Isolated {
			base = image.NewRGBA(rr.out.Bounds())
			baseRelease, err := rr.acquire(int64(len(base.Pix)))
			if err != nil {
				return err
			}
			defer baseRelease()
			draw.Draw(base, base.Bounds(), rr.out, image.Point{}, draw.Src)
			draw.Draw(off, off.Bounds(), base, image.Point{}, draw.Src)
		}
		for _, it := range g.Items {
			if rr.opts.Exclude != nil && rr.opts.Exclude(it) {
				continue
			}
			// Render each element without the group backdrop. Only pixels
			// with non-zero element alpha replace the group result; transparent
			// holes must not erase earlier elements.
			item := image.NewRGBA(rr.out.Bounds())
			itemRelease, err := rr.acquire(int64(len(item.Pix)))
			if err != nil {
				return err
			}
			child := &regionRenderer{
				ctx: rr.ctx, dl: rr.dl, out: item, box: rr.box, scale: rr.scale,
				pageToPx: rr.pageToPx, opts: rr.opts, clips: map[int]*image.Alpha{},
				clipList: g.Clips, softMasks: map[*displaylist.SoftMask]*image.Alpha{},
				memory: rr.memory, gray: rr.gray || grayBlendSpace(g.BlendColorSpace),
			}
			err = child.renderItem(it)
			child.releaseAll()
			if err != nil {
				itemRelease()
				return err
			}
			// For non-Normal blends, render a second copy over the initial
			// backdrop so the blend mode sees the correct destination. The
			// transparent item still supplies the coverage mask.
			var blended *image.RGBA
			var blendedRelease func()
			blendMode := it.Base().Blend
			if base != nil && blendMode != "" && !strings.EqualFold(blendMode, "Normal") {
				blended = image.NewRGBA(rr.out.Bounds())
				if blendedRelease, err = rr.acquire(int64(len(blended.Pix))); err != nil {
					itemRelease()
					return err
				}
				draw.Draw(blended, blended.Bounds(), base, image.Point{}, draw.Src)
				bchild := &regionRenderer{
					ctx: rr.ctx, dl: rr.dl, out: blended, box: rr.box, scale: rr.scale,
					pageToPx: rr.pageToPx, opts: rr.opts, clips: map[int]*image.Alpha{},
					clipList: g.Clips, softMasks: map[*displaylist.SoftMask]*image.Alpha{},
					memory: rr.memory, gray: rr.gray || grayBlendSpace(g.BlendColorSpace),
				}
				err = bchild.renderItem(it)
				bchild.releaseAll()
				if err != nil {
					blendedRelease()
					itemRelease()
					return err
				}
			}
			b := it.Base().Bounds
			dst := pxRect(b, rr.pageToPx).Intersect(off.Bounds())
			for y := dst.Min.Y; y < dst.Max.Y; y++ {
				for x := dst.Min.X; x < dst.Max.X; x++ {
					sp := item.RGBAAt(x, y)
					sa := float64(sp.A) / 255
					if sa <= 0 {
						continue
					}
					if base == nil {
						off.SetRGBA(x, y, sp)
						continue
					}
					if blended != nil {
						off.SetRGBA(x, y, blended.RGBAAt(x, y))
						continue
					}
					bp := base.RGBAAt(x, y)
					ba := float64(bp.A) / 255
					oa := sa + ba*(1-sa)
					if oa <= 0 {
						continue
					}
					sr, sg, sb := float64(sp.R)/255/sa, float64(sp.G)/255/sa, float64(sp.B)/255/sa
					br, bg, bb0 := 0.0, 0.0, 0.0
					if ba > 0 {
						br, bg, bb0 = float64(bp.R)/255/ba, float64(bp.G)/255/ba, float64(bp.B)/255/ba
					}
					r := (sr*sa + br*ba*(1-sa)) / oa
					gg := (sg*sa + bg*ba*(1-sa)) / oa
					bb := (sb*sa + bb0*ba*(1-sa)) / oa
					off.SetRGBA(x, y, color.RGBA{
						R: uint8(clamp01(r) * oa * 255),
						G: uint8(clamp01(gg) * oa * 255),
						B: uint8(clamp01(bb) * oa * 255),
						A: uint8(clamp01(oa) * 255),
					})
				}
			}
			if blendedRelease != nil {
				blendedRelease()
			}
			itemRelease()
		}
		if g.Alpha < 1 {
			scaleAlphaRGBA(off, g.Alpha)
		}
		rr.composite(off, image.Point{}, g.Clip, g.Blend)
		return nil
	}
	// A non-isolated group blends its contents directly with the current
	// backdrop. This approximates the PDF group model without an offscreen
	// backdrop copy; isolated groups are rendered offscreen and composited.
	if !g.Isolated && !g.Knockout && g.Alpha >= 1 {
		off := image.NewRGBA(rr.out.Bounds())
		release, err := rr.acquire(int64(len(off.Pix)))
		if err != nil {
			return err
		}
		defer release()
		if g.Backdrop != nil {
			draw.Draw(off, off.Bounds(), image.NewUniform(rr.solidColor(g.Backdrop.Color, g.Backdrop.Color[3])), image.Point{}, draw.Src)
		} else {
			draw.Draw(off, off.Bounds(), rr.out, image.Point{}, draw.Src)
		}
		child := &regionRenderer{
			ctx: rr.ctx, dl: rr.dl, out: off, box: rr.box, scale: rr.scale,
			pageToPx: rr.pageToPx, opts: rr.opts, clips: map[int]*image.Alpha{},
			clipList: g.Clips, softMasks: map[*displaylist.SoftMask]*image.Alpha{},
			softMask: rr.softMask, memory: rr.memory, gray: rr.gray || grayBlendSpace(g.BlendColorSpace),
		}
		defer child.releaseAll()
		for _, it := range g.Items {
			if rr.opts.Exclude != nil && rr.opts.Exclude(it) {
				continue
			}
			if err := child.renderItem(it); err != nil {
				return err
			}
		}
		bbox := g.Bounds
		if bbox.X1 <= bbox.X0 || bbox.Y1 <= bbox.Y0 {
			bbox = displaylist.Rect{X0: rr.box.X0, Y0: rr.box.Y0, X1: rr.box.X1, Y1: rr.box.Y1}
		}
		if grayBlendSpace(g.BlendColorSpace) {
			toGrayRGBA(off)
		}
		dst := pxRect(bbox, rr.pageToPx).Intersect(rr.out.Bounds())
		if dst.Empty() {
			return nil
		}
		if mask := rr.clipMask(g.Clip); mask != nil {
			draw.DrawMask(rr.out, dst, off, dst.Min, mask, dst.Min, draw.Src)
		} else {
			draw.Draw(rr.out, dst, off, dst.Min, draw.Src)
		}
		return nil
	}
	off := image.NewRGBA(rr.out.Bounds())
	release, err := rr.acquire(int64(len(off.Pix)))
	if err != nil {
		return err
	}
	defer release()
	if g.Backdrop != nil {
		draw.Draw(off, off.Bounds(), image.NewUniform(rr.solidColor(g.Backdrop.Color, g.Backdrop.Color[3])), image.Point{}, draw.Src)
	}
	child := &regionRenderer{
		ctx: rr.ctx, dl: rr.dl, out: off, box: rr.box, scale: rr.scale,
		pageToPx: rr.pageToPx, opts: rr.opts, clips: map[int]*image.Alpha{},
		clipList: g.Clips, softMasks: map[*displaylist.SoftMask]*image.Alpha{},
		memory: rr.memory, gray: rr.gray || grayBlendSpace(g.BlendColorSpace),
	}
	defer child.releaseAll()
	for _, it := range g.Items {
		if rr.opts.Exclude != nil && rr.opts.Exclude(it) {
			continue
		}
		if err := child.renderItem(it); err != nil {
			return err
		}
	}
	if g.Isolated && grayBlendSpace(g.BlendColorSpace) {
		toGrayRGBA(off)
	}
	if g.Alpha < 1 {
		scaleAlphaRGBA(off, g.Alpha)
	}
	rr.composite(off, image.Point{}, g.Clip, g.Blend)
	return nil
}

func scaleAlphaRGBA(img *image.RGBA, alpha float64) {
	for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y++ {
		for x := img.Rect.Min.X; x < img.Rect.Max.X; x++ {
			img.SetRGBA(x, y, scaleColor(img.RGBAAt(x, y), alpha))
		}
	}
}

func grayBlendSpace(name string) bool {
	return name == "DeviceGray" || name == "G" || name == "Gray"
}

func toGrayRGBA(img *image.RGBA) {
	for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y++ {
		row := img.Pix[img.PixOffset(img.Rect.Min.X, y):img.PixOffset(img.Rect.Max.X, y)]
		for i := 0; i+3 < len(row); i += 4 {
			l := uint8((299*int(row[i]) + 587*int(row[i+1]) + 114*int(row[i+2]) + 500) / 1000)
			row[i], row[i+1], row[i+2] = l, l, l
		}
	}
}

func fillAlpha(c *displaylist.Common) float64 {
	if c != nil && c.PaintState != nil {
		return c.PaintState.FillAlpha
	}
	if c != nil {
		return c.Alpha
	}
	return 1
}

func strokeAlpha(c *displaylist.Common) float64 {
	if c != nil && c.PaintState != nil {
		return c.PaintState.StrokeAlpha
	}
	if c != nil {
		return c.Alpha
	}
	return 1
}

type regionRenderer struct {
	ctx       context.Context
	dl        *displaylist.Page
	out       *image.RGBA
	box       displaylist.Rect
	scale     float64
	pageToPx  displaylist.Matrix
	opts      PageOptions
	clips     map[int]*image.Alpha
	clipList  []displaylist.Clip
	mask      *image.Alpha
	softMask  *image.Alpha
	softMasks map[*displaylist.SoftMask]*image.Alpha
	inMask    bool
	memory    MemoryBudget
	memErr    error
	gray      bool
	leases    []func()
}

func (rr *regionRenderer) acquire(n int64) (func(), error) {
	if rr.memory == nil || n <= 0 {
		return func() {}, nil
	}
	return rr.memory.AcquireMemory(n)
}

func (rr *regionRenderer) releaseAll() {
	for i := len(rr.leases) - 1; i >= 0; i-- {
		rr.leases[i]()
	}
	rr.leases = nil
}

func (rr *regionRenderer) path(p *displaylist.Path) error {
	if p == nil || (p.Fill == nil && p.Stroke == nil) {
		return nil
	}
	bbox := pxRect(p.Bounds, rr.pageToPx)
	if p.Stroke != nil {
		pad := p.Line.Width * rr.scale / 2
		bbox = image.Rect(
			int(math.Floor(float64(bbox.Min.X)-pad)),
			int(math.Floor(float64(bbox.Min.Y)-pad)),
			int(math.Ceil(float64(bbox.Max.X)+pad)),
			int(math.Ceil(float64(bbox.Max.Y)+pad)),
		)
	}
	bbox = bbox.Intersect(rr.out.Bounds())
	if bbox.Empty() {
		return nil
	}
	tmp := image.NewRGBA(image.Rect(0, 0, bbox.Dx(), bbox.Dy()))
	release, err := rr.acquire(int64(len(tmp.Pix)))
	if err != nil {
		return err
	}
	defer release()
	offset := image.Pt(bbox.Min.X, bbox.Min.Y)
	if p.Fill != nil && p.Fill.Kind == displaylist.PaintPattern && p.Fill.Pattern != nil {
		mask := rr.pathMask(p)
		if p.Fill.Pattern.Type == 2 && p.Fill.Pattern.Shading != nil {
			rr.shadingPattern(p, mask)
		} else {
			rr.pattern(p, mask)
		}
	} else if p.Fill != nil && p.Fill.Kind == displaylist.PaintSolid {
		rr.fill(tmp, p.Segments, p.FillRule, p.Closed, rr.solidColor(p.Fill.RGBA, fillAlpha(&p.Common)), offset)
	}
	if p.Stroke != nil && p.Stroke.Kind == displaylist.PaintSolid {
		rr.stroke(tmp, p.Segments, p.Stroke, strokeAlpha(&p.Common), offset, p.Line, p.Closed)
	}
	rr.composite(tmp, bbox.Min, p.Clip, p.Blend)
	return nil
}

func (rr *regionRenderer) text(t *displaylist.TextRun) error {
	if t == nil || t.Font == nil || len(t.Glyphs) == 0 {
		return nil
	}
	if t.Font.GlyphSource == nil && rr.opts.Fonts == nil {
		return nil
	}
	mode := t.RenderMode
	if mode < 0 || mode > 7 {
		mode = 0
	}
	if mode == 3 || t.TextFlags&displaylist.TextInvisible != 0 {
		return nil
	}
	fillMode := mode == 0 || mode == 2
	strokeMode := mode == 1 || mode == 2
	if !fillMode && !strokeMode {
		// Text clipping modes (4-7) are not yet represented as a renderable
		// text clip surface.
		return nil
	}
	bbox := pxRect(t.Bounds, rr.pageToPx)
	pad := int(math.Ceil(t.Size * rr.scale))
	if strokeMode {
		pad += int(math.Ceil(t.Line.Width * rr.scale / 2))
	}
	bbox = image.Rect(bbox.Min.X-pad, bbox.Min.Y-pad, bbox.Max.X+pad, bbox.Max.Y+pad)
	bbox = bbox.Intersect(rr.out.Bounds())
	if bbox.Empty() {
		return nil
	}
	tmp := image.NewRGBA(image.Rect(0, 0, bbox.Dx(), bbox.Dy()))
	release, err := rr.acquire(int64(len(tmp.Pix)))
	if err != nil {
		return err
	}
	defer release()
	offset := image.Pt(bbox.Min.X, bbox.Min.Y)
	var fillR, strokeR vector.Rasterizer
	if fillMode {
		fillR.Reset(tmp.Bounds().Dx(), tmp.Bounds().Dy())
	}
	if strokeMode {
		strokeR.Reset(tmp.Bounds().Dx(), tmp.Bounds().Dy())
	}
	added := false
	for _, g := range t.Glyphs {
		var seg []displaylist.Segment
		var ok bool
		if t.Font.GlyphSource != nil {
			seg, ok = t.Font.GlyphSource.GlyphOutline(g.Code, g.CID)
		}
		if !ok && rr.opts.Fonts != nil {
			var err error
			seg, ok, err = rr.opts.Fonts.GlyphOutline(t.Font, g)
			if err != nil {
				return err
			}
		}
		if !ok {
			continue
		}
		const glyphScale = 0.001
		m := mul(rr.pageToPx, mul(g.Matrix, displaylist.Matrix{glyphScale, 0, 0, glyphScale, 0, 0}))
		if fillMode {
			addSegments(&fillR, seg, m, offset)
		}
		if strokeMode {
			for _, pts := range flattenPath(seg, m, offset) {
				strokePoints(&strokeR, pts, t.Line.Width*rr.scale/2, textLine(t.Line, rr.scale), true)
			}
		}
		added = true
	}
	if !added {
		return nil
	}
	if fillMode {
		fill := displaylist.Paint{RGBA: [4]float64{0, 0, 0, 1}}
		if t.Fill.Kind == displaylist.PaintSolid {
			fill = t.Fill
		}
		fillR.Draw(tmp, tmp.Bounds(), image.NewUniform(rr.solidColor(fill.RGBA, fillAlpha(&t.Common))), image.Point{})
	}
	if strokeMode {
		stroke := displaylist.Paint{RGBA: [4]float64{0, 0, 0, 1}}
		if t.Stroke != nil && t.Stroke.Kind == displaylist.PaintSolid {
			stroke = *t.Stroke
		}
		strokeR.Draw(tmp, tmp.Bounds(), image.NewUniform(rr.solidColor(stroke.RGBA, strokeAlpha(&t.Common))), image.Point{})
	}
	rr.composite(tmp, bbox.Min, t.Clip, t.Blend)
	return nil
}

func textLine(line displaylist.LineStyle, scale float64) displaylist.LineStyle {
	if scale != 1 && scale > 0 {
		if len(line.Dash) > 0 {
			dash := make([]float64, len(line.Dash))
			for i, d := range line.Dash {
				dash[i] = d * scale
			}
			line.Dash = dash
		}
		line.Phase *= scale
	}
	return line
}

type leasedDecoder interface {
	DecodeLease(context.Context) (image.Image, func(), error)
}

func decodeImage(ctx context.Context, ref displaylist.ImageRef) (image.Image, func(), error) {
	if l, ok := ref.(leasedDecoder); ok {
		return l.DecodeLease(ctx)
	}
	img, err := ref.Decode(ctx)
	return img, func() {}, err
}

func (rr *regionRenderer) image(im *displaylist.Image) (err error) {
	if im == nil || im.Data == nil {
		return nil
	}
	isMask := im.Info.ColorSpace == "ImageMask"
	decoded, release, err := decodeImage(rr.ctx, im.Data)
	if release != nil {
		defer release()
	}
	if rr.gray && decoded != nil {
		decoded = grayImage{decoded}
	}
	if err != nil {
		if errors.Is(err, errs.ErrUnsupported) {
			return errs.Wrap(errs.ErrUnsupported, "RND-IMAGE", "decode image", -1, 0, -1)
		}
		if errors.Is(err, errs.ErrBudget) || errors.Is(err, errs.ErrCanceled) {
			return err
		}
		return errs.Wrap(errs.ErrCorrupt, "RND-IMAGE", "decode image", -1, 0, -1)
	}
	var maskImg image.Image
	if im.Mask != nil && im.Mask.Image != nil {
		var maskRelease func()
		maskImg, maskRelease, _ = decodeImage(rr.ctx, im.Mask.Image)
		if maskRelease != nil {
			defer maskRelease()
		}
	}
	bbox := imageRect(im.Matrix, rr.pageToPx).Intersect(rr.out.Bounds())
	if bbox.Empty() {
		return nil
	}
	inv, ok := invert(im.Matrix)
	if !ok {
		return nil
	}
	clip := rr.clipMask(im.Clip)
	sb := decoded.Bounds()
	sw, sh := float64(sb.Dx()), float64(sb.Dy())
	alpha := fillAlpha(&im.Common)
	for y := bbox.Min.Y; y < bbox.Max.Y; y++ {
		for x := bbox.Min.X; x < bbox.Max.X; x++ {
			p := displaylist.Point{X: rr.box.X0 + (float64(x)+0.5)/rr.scale, Y: rr.box.Y0 + (float64(y)+0.5)/rr.scale}
			q := apply(inv, p.X, p.Y)
			u, v := q.X, q.Y
			if u < 0 || u >= 1 || v < 0 || v >= 1 {
				continue
			}
			sx := sb.Min.X + int(u*sw)
			sy := sb.Min.Y + int((1-v)*sh)
			if sx < sb.Min.X || sx >= sb.Max.X || sy < sb.Min.Y || sy >= sb.Max.Y {
				continue
			}
			var r, g, b, a uint32
			oldA := uint32(0)
			if isMask {
				_, _, _, ma := decoded.At(sx, sy).RGBA()
				fill := [4]float64{0, 0, 0, 1}
				if im.Fill != nil && im.Fill.Kind == displaylist.PaintSolid {
					fill = im.Fill.RGBA
				}
				src := rr.solidColor(fill, alpha)
				src = scaleColor(src, float64(ma)/65535)
				r, g, b, a = uint32(src.R)*0x101, uint32(src.G)*0x101, uint32(src.B)*0x101, uint32(src.A)*0x101
				oldA = a
			} else {
				r, g, b, a = decoded.At(sx, sy).RGBA()
				oldA = a
				a = uint32(float64(a) * alpha)
				if im.Mask != nil {
					if im.Mask.IsColorKey {
						a = uint32(float64(a) * colorKeyAlpha(im, r, g, b))
					} else {
						a = uint32(float64(a) * rr.maskAlpha(im, maskImg, u, v))
					}
				}
			}
			if clip != nil {
				a = uint32(float64(a) * float64(clip.AlphaAt(x, y).A) / 255)
			}
			if rr.mask != nil {
				a = uint32(float64(a) * float64(rr.mask.AlphaAt(x, y).A) / 255)
			}
			if rr.softMask != nil {
				a = uint32(float64(a) * float64(rr.softMask.AlphaAt(x, y).A) / 255)
			}
			if oldA > 0 {
				r = uint32(float64(r) * float64(a) / float64(oldA))
				g = uint32(float64(g) * float64(a) / float64(oldA))
				b = uint32(float64(b) * float64(a) / float64(oldA))
			}
			rr.blend(x, y, r, g, b, a, im.Blend)
		}
	}
	return nil
}

func (rr *regionRenderer) maskAlpha(im *displaylist.Image, mask image.Image, u, v float64) float64 {
	if im.Mask == nil {
		return 1
	}
	if im.Mask.IsColorKey {
		if len(im.Mask.ColorKey) < 2 {
			return 1
		}
		return 1
	}
	if mask == nil {
		return 1
	}
	mb := mask.Bounds()
	x := mb.Min.X + int(u*float64(mb.Dx()))
	y := mb.Min.Y + int((1-v)*float64(mb.Dy()))
	if x < mb.Min.X || x >= mb.Max.X || y < mb.Min.Y || y >= mb.Max.Y {
		return 1
	}
	r, _, _, _ := mask.At(x, y).RGBA()
	return float64(r) / 65535
}

func (rr *regionRenderer) shading(s *displaylist.Shading) error {
	if s == nil {
		return nil
	}
	if s.Kind == displaylist.ShadingMesh {
		return rr.mesh(s)
	}
	if s.Kind == displaylist.ShadingFunction && s.Grid != nil {
		return rr.functionShading(s)
	}
	if (s.Kind != displaylist.ShadingAxial && s.Kind != displaylist.ShadingRadial) || len(s.Stops) < 2 {
		return nil
	}
	geom, ok := s.Geometry.(displaylist.ShadingGeometry)
	if !ok {
		return nil
	}
	bbox := pxRect(s.Bounds, rr.pageToPx).Intersect(rr.out.Bounds())
	if bbox.Empty() {
		return nil
	}
	clip := rr.clipMask(s.Clip)
	for y := bbox.Min.Y; y < bbox.Max.Y; y++ {
		for x := bbox.Min.X; x < bbox.Max.X; x++ {
			p := displaylist.Point{X: rr.box.X0 + (float64(x)+0.5)/rr.scale, Y: rr.box.Y0 + (float64(y)+0.5)/rr.scale}
			var t float64
			if s.Kind == displaylist.ShadingAxial {
				dx, dy := geom.X1-geom.X0, geom.Y1-geom.Y0
				den := dx*dx + dy*dy
				if den == 0 {
					continue
				}
				t = ((p.X-geom.X0)*dx + (p.Y-geom.Y0)*dy) / den
			} else {
				den := geom.R1 - geom.R0
				if den == 0 {
					continue
				}
				t = (math.Hypot(p.X-geom.X0, p.Y-geom.Y0) - geom.R0) / den
			}
			if t < 0 {
				if !s.Extend[0] {
					continue
				}
				t = 0
			}
			if t > 1 {
				if !s.Extend[1] {
					continue
				}
				t = 1
			}
			c := shadingColor(s.Stops, t)
			a := fillAlpha(&s.Common) * float64(c.A) / 255
			if clip != nil {
				a *= float64(clip.AlphaAt(x, y).A) / 255
			}
			if rr.mask != nil {
				a *= float64(rr.mask.AlphaAt(x, y).A) / 255
			}
			if rr.softMask != nil {
				a *= float64(rr.softMask.AlphaAt(x, y).A) / 255
			}
			rr.blend(x, y, uint32(float64(c.R)*0x101*a), uint32(float64(c.G)*0x101*a), uint32(float64(c.B)*0x101*a), uint32(a*65535), s.Blend)
		}
	}
	return nil
}

func (rr *regionRenderer) pathMask(p *displaylist.Path) *image.Alpha {
	m := image.NewAlpha(rr.out.Bounds())
	release, err := rr.acquire(int64(len(m.Pix)))
	if err != nil {
		rr.memErr = err
		return nil
	}
	rr.leases = append(rr.leases, release)
	if p.FillRule == displaylist.EvenOdd {
		tmp := image.NewRGBA(rr.out.Bounds())
		tmpRelease, err := rr.acquire(int64(len(tmp.Pix)))
		if err != nil {
			rr.memErr = err
			return nil
		}
		defer tmpRelease()
		paths := flattenPath(p.Segments, rr.pageToPx, image.Point{})
		if p.Closed {
			for i := range paths {
				if len(paths[i]) > 0 && paths[i][0] != paths[i][len(paths[i])-1] {
					paths[i] = append(paths[i], paths[i][0])
				}
			}
		}
		fillEvenOdd(tmp, paths, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		for y := 0; y < m.Bounds().Dy(); y++ {
			for x := 0; x < m.Bounds().Dx(); x++ {
				m.SetAlpha(x, y, color.Alpha{A: tmp.RGBAAt(x, y).A})
			}
		}
	} else {
		var r vector.Rasterizer
		r.Reset(m.Bounds().Dx(), m.Bounds().Dy())
		addSegments(&r, p.Segments, rr.pageToPx, image.Point{})
		r.Draw(m, m.Bounds(), image.NewUniform(color.Alpha{A: 255}), image.Point{})
	}
	if p.Clip >= 0 {
		if cm := rr.clipMask(p.Clip); cm != nil {
			for y := 0; y < m.Bounds().Dy(); y++ {
				for x := 0; x < m.Bounds().Dx(); x++ {
					m.SetAlpha(x, y, color.Alpha{A: uint8(float64(m.AlphaAt(x, y).A) * float64(cm.AlphaAt(x, y).A) / 255)})
				}
			}
		}
	}
	if a := fillAlpha(&p.Common); a < 1 {
		for y := 0; y < m.Bounds().Dy(); y++ {
			for x := 0; x < m.Bounds().Dx(); x++ {
				m.SetAlpha(x, y, color.Alpha{A: uint8(float64(m.AlphaAt(x, y).A) * a)})
			}
		}
	}
	return m
}

func (rr *regionRenderer) shadingPattern(p *displaylist.Path, mask *image.Alpha) {
	pat := p.Fill.Pattern
	if pat == nil || pat.Shading == nil {
		return
	}
	child := &regionRenderer{
		ctx: rr.ctx, dl: rr.dl, out: rr.out, box: rr.box, scale: rr.scale,
		pageToPx: rr.pageToPx, opts: rr.opts,
		clips: map[int]*image.Alpha{}, clipList: pat.Clips, mask: mask,
		softMask: rr.softMask, softMasks: map[*displaylist.SoftMask]*image.Alpha{},
		memory: rr.memory, gray: rr.gray,
	}
	_ = child.renderItem(pat.Shading)
	child.releaseAll()
}

func (rr *regionRenderer) pattern(p *displaylist.Path, mask *image.Alpha) {
	pat := p.Fill.Pattern
	if pat == nil || len(pat.Items) == 0 || pat.XStep == 0 && pat.YStep == 0 {
		return
	}
	origin := apply(pat.Matrix, 0, 0)
	px := apply(pat.Matrix, pat.XStep, 0)
	py := apply(pat.Matrix, 0, pat.YStep)
	stepX := displaylist.Point{X: px.X - origin.X, Y: px.Y - origin.Y}
	stepY := displaylist.Point{X: py.X - origin.X, Y: py.Y - origin.Y}
	toTile := displaylist.Matrix{stepX.X, stepX.Y, stepY.X, stepY.Y, origin.X, origin.Y}
	inv, ok := invert(toTile)
	if !ok {
		return
	}
	minI, minJ, maxI, maxJ := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, q := range []displaylist.Point{{X: p.Bounds.X0, Y: p.Bounds.Y0}, {X: p.Bounds.X1, Y: p.Bounds.Y0}, {X: p.Bounds.X1, Y: p.Bounds.Y1}, {X: p.Bounds.X0, Y: p.Bounds.Y1}} {
		t := apply(inv, q.X, q.Y)
		minI, minJ = math.Min(minI, t.X), math.Min(minJ, t.Y)
		maxI, maxJ = math.Max(maxI, t.X), math.Max(maxJ, t.Y)
	}
	for j := int(math.Floor(minJ)) - 1; j <= int(math.Ceil(maxJ))+1; j++ {
		for i := int(math.Floor(minI)) - 1; i <= int(math.Ceil(maxI))+1; i++ {
			off := displaylist.Point{X: float64(i)*stepX.X + float64(j)*stepY.X, Y: float64(i)*stepX.Y + float64(j)*stepY.Y}
			child := &regionRenderer{
				ctx: rr.ctx, dl: rr.dl, out: rr.out, box: rr.box, scale: rr.scale,
				pageToPx: mul(rr.pageToPx, translate(off.X, off.Y)), opts: rr.opts,
				clips: map[int]*image.Alpha{}, clipList: pat.Clips, mask: mask,
				softMask: rr.softMask, softMasks: map[*displaylist.SoftMask]*image.Alpha{},
				memory: rr.memory, gray: rr.gray,
			}
			for _, it := range pat.Items {
				if rr.opts.Exclude != nil && rr.opts.Exclude(it) {
					continue
				}
				if err := child.renderItem(it); err != nil {
					return
				}
			}
		}
	}
}

func (rr *regionRenderer) fill(dst *image.RGBA, seg []displaylist.Segment, rule displaylist.FillRule, closed bool, c color.RGBA, offset image.Point) {
	if rule == displaylist.EvenOdd {
		paths := flattenPath(seg, rr.pageToPx, offset)
		if closed {
			for i := range paths {
				if len(paths[i]) > 0 && paths[i][0] != paths[i][len(paths[i])-1] {
					paths[i] = append(paths[i], paths[i][0])
				}
			}
		}
		fillEvenOdd(dst, paths, c)
		return
	}
	var r vector.Rasterizer
	r.Reset(dst.Bounds().Dx(), dst.Bounds().Dy())
	addSegments(&r, seg, rr.pageToPx, offset)
	r.Draw(dst, dst.Bounds(), image.NewUniform(c), image.Point{})
}

func (rr *regionRenderer) stroke(dst *image.RGBA, seg []displaylist.Segment, paint *displaylist.Paint, alpha float64, offset image.Point, line displaylist.LineStyle, closed bool) {
	half := line.Width * rr.scale / 2
	if half < 0.5 {
		half = 0.5
	}
	if paint == nil || paint.Kind != displaylist.PaintSolid || half <= 0 {
		return
	}
	var r vector.Rasterizer
	r.Reset(dst.Bounds().Dx(), dst.Bounds().Dy())
	if len(line.Dash) > 0 {
		dash := make([]float64, len(line.Dash))
		for i, d := range line.Dash {
			dash[i] = d * rr.scale
		}
		line.Dash = dash
		line.Phase *= rr.scale
	}
	for _, pts := range flattenPath(seg, rr.pageToPx, offset) {
		strokePoints(&r, pts, half, line, closed)
	}
	r.Draw(dst, dst.Bounds(), image.NewUniform(rr.solidColor(paint.RGBA, alpha)), image.Point{})
}

func (rr *regionRenderer) composite(src *image.RGBA, at image.Point, clip int, mode string) {
	dst := image.Rect(at.X, at.Y, at.X+src.Bounds().Dx(), at.Y+src.Bounds().Dy())
	mask := rr.clipMask(clip)
	if rr.mask != nil || rr.softMask != nil || mask != nil || (mode != "" && !strings.EqualFold(mode, "Normal")) {
		for y := dst.Min.Y; y < dst.Max.Y; y++ {
			for x := dst.Min.X; x < dst.Max.X; x++ {
				c := src.RGBAAt(x-at.X, y-at.Y)
				if mask != nil {
					c = scaleColor(c, float64(mask.AlphaAt(x, y).A)/255)
				}
				if rr.mask != nil {
					c = scaleColor(c, float64(rr.mask.AlphaAt(x, y).A)/255)
				}
				if rr.softMask != nil {
					c = scaleColor(c, float64(rr.softMask.AlphaAt(x, y).A)/255)
				}
				rr.blendRGBA(x, y, c, mode)
			}
		}
		return
	}
	draw.Draw(rr.out, dst, src, src.Bounds().Min, draw.Over)
}

func (rr *regionRenderer) clipMask(id int) *image.Alpha {
	if id < 0 || id >= len(rr.clipList) {
		return nil
	}
	if m, ok := rr.clips[id]; ok {
		return m
	}
	m, release := rr.clipValue(rr.clipList[id])
	if m == nil {
		return nil
	}
	rr.leases = append(rr.leases, release)
	rr.clips[id] = m
	return m
}

func (rr *regionRenderer) clipValue(c displaylist.Clip) (*image.Alpha, func()) {
	m := image.NewAlpha(rr.out.Bounds())
	release, err := rr.acquire(int64(len(m.Pix)))
	if err != nil {
		rr.memErr = err
		return nil, func() {}
	}
	if c.Rule == displaylist.EvenOdd {
		tmp := image.NewRGBA(rr.out.Bounds())
		tmpRelease, err := rr.acquire(int64(len(tmp.Pix)))
		if err != nil {
			rr.memErr = err
			release()
			return nil, func() {}
		}
		fillEvenOdd(tmp, flattenPath(c.Path, rr.pageToPx, image.Point{}), color.RGBA{R: 255, G: 255, B: 255, A: 255})
		for y := 0; y < rr.out.Bounds().Dy(); y++ {
			for x := 0; x < rr.out.Bounds().Dx(); x++ {
				m.SetAlpha(x, y, color.Alpha{A: tmp.RGBAAt(x, y).A})
			}
		}
		tmpRelease()
	} else {
		var r vector.Rasterizer
		r.Reset(rr.out.Bounds().Dx(), rr.out.Bounds().Dy())
		addSegments(&r, c.Path, rr.pageToPx, image.Point{})
		r.Draw(m, m.Bounds(), image.NewUniform(color.Alpha{A: 255}), image.Point{})
	}
	for _, sub := range c.Intersect {
		sm, subRelease := rr.clipValue(sub)
		if sm == nil {
			continue
		}
		for y := 0; y < m.Bounds().Dy(); y++ {
			for x := 0; x < m.Bounds().Dx(); x++ {
				m.SetAlpha(x, y, color.Alpha{A: uint8(float64(m.AlphaAt(x, y).A) * float64(sm.AlphaAt(x, y).A) / 255)})
			}
		}
		subRelease()
	}
	return m, release
}

func translate(x, y float64) displaylist.Matrix {
	return displaylist.Matrix{1, 0, 0, 1, x, y}
}

func (rr *regionRenderer) blend(x, y int, r, g, b, a uint32, mode string) {
	if a == 0 {
		return
	}
	rr.blendRGBA(x, y, color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}, mode)
}

func (rr *regionRenderer) blendRGBA(x, y int, src color.RGBA, mode string) {
	if src.A == 0 {
		return
	}
	dst := rr.out.RGBAAt(x, y)
	as := float64(src.A) / 255
	ad := float64(dst.A) / 255
	cs := [3]float64{unpremul(src.R, src.A), unpremul(src.G, src.A), unpremul(src.B, src.A)}
	cb := [3]float64{unpremul(dst.R, dst.A), unpremul(dst.G, dst.A), unpremul(dst.B, dst.A)}
	bl := blendColor(mode, cb, cs)
	ao := as + ad - as*ad
	if ao <= 0 {
		return
	}
	out := [3]float64{}
	for i := range out {
		out[i] = as*(1-ad)*cs[i] + as*ad*bl[i] + (1-as)*ad*cb[i]
	}
	A := uint8(clamp01(ao) * 255)
	R := uint8(clamp01(out[0]) * 255)
	G := uint8(clamp01(out[1]) * 255)
	B := uint8(clamp01(out[2]) * 255)
	if R > A {
		R = A
	}
	if G > A {
		G = A
	}
	if B > A {
		B = A
	}
	rr.out.SetRGBA(x, y, color.RGBA{R: R, G: G, B: B, A: A})
}

func unpremul(v, a uint8) float64 {
	if a == 0 {
		return 0
	}
	return float64(v) / float64(a)
}

func scaleColor(c color.RGBA, f float64) color.RGBA {
	if f <= 0 {
		return color.RGBA{}
	}
	if f > 1 {
		f = 1
	}
	return color.RGBA{uint8(float64(c.R) * f), uint8(float64(c.G) * f), uint8(float64(c.B) * f), uint8(float64(c.A) * f)}
}

func colorKeyAlpha(im *displaylist.Image, r, g, b uint32) float64 {
	key := im.Mask.ColorKey
	n := 3
	vals := []float64{float64(r) / 65535, float64(g) / 65535, float64(b) / 65535}
	switch im.Info.ColorSpace {
	case "DeviceGray":
		n, vals = 1, vals[:1]
	case "DeviceCMYK":
		return 1
	}
	if len(key) < n*2 {
		return 1
	}
	for i := 0; i < n; i++ {
		if vals[i] < key[2*i] || vals[i] > key[2*i+1] {
			return 1
		}
	}
	return 0
}

func blendColor(mode string, cb, cs [3]float64) [3]float64 {
	switch strings.ToLower(mode) {
	case "", "normal":
		return cs
	case "multiply":
		return [3]float64{cb[0] * cs[0], cb[1] * cs[1], cb[2] * cs[2]}
	case "screen":
		return [3]float64{1 - (1-cb[0])*(1-cs[0]), 1 - (1-cb[1])*(1-cs[1]), 1 - (1-cb[2])*(1-cs[2])}
	case "overlay":
		return [3]float64{hardLight(cs[0], cb[0]), hardLight(cs[1], cb[1]), hardLight(cs[2], cb[2])}
	case "darken":
		return [3]float64{math.Min(cb[0], cs[0]), math.Min(cb[1], cs[1]), math.Min(cb[2], cs[2])}
	case "lighten":
		return [3]float64{math.Max(cb[0], cs[0]), math.Max(cb[1], cs[1]), math.Max(cb[2], cs[2])}
	case "colordodge":
		return [3]float64{dodge(cb[0], cs[0]), dodge(cb[1], cs[1]), dodge(cb[2], cs[2])}
	case "colorburn":
		return [3]float64{burn(cb[0], cs[0]), burn(cb[1], cs[1]), burn(cb[2], cs[2])}
	case "hardlight":
		return [3]float64{hardLight(cb[0], cs[0]), hardLight(cb[1], cs[1]), hardLight(cb[2], cs[2])}
	case "softlight":
		return [3]float64{softLight(cb[0], cs[0]), softLight(cb[1], cs[1]), softLight(cb[2], cs[2])}
	case "difference":
		return [3]float64{math.Abs(cb[0] - cs[0]), math.Abs(cb[1] - cs[1]), math.Abs(cb[2] - cs[2])}
	case "exclusion":
		return [3]float64{cb[0] + cs[0] - 2*cb[0]*cs[0], cb[1] + cs[1] - 2*cb[1]*cs[1], cb[2] + cs[2] - 2*cb[2]*cs[2]}
	case "hue", "saturation", "color", "luminosity":
		h, s, l := rgbToHSL(cb)
		hs, ss, ls := rgbToHSL(cs)
		switch strings.ToLower(mode) {
		case "hue":
			h = hs
		case "saturation":
			s = ss
		case "color":
			h, s = hs, ss
		case "luminosity":
			l = ls
		}
		return hslToRGB(h, s, l)
	default:
		return cs
	}
}

func hardLight(cb, cs float64) float64 {
	if cs <= 0.5 {
		return 2 * cb * cs
	}
	return 1 - 2*(1-cb)*(1-cs)
}

func dodge(cb, cs float64) float64 {
	if cb == 0 {
		return 0
	}
	if cs >= 1 {
		return 1
	}
	return math.Min(1, cb/(1-cs))
}

func burn(cb, cs float64) float64 {
	if cb >= 1 {
		return 1
	}
	if cs <= 0 {
		return 0
	}
	return 1 - math.Min(1, (1-cb)/cs)
}

func softLight(cb, cs float64) float64 {
	if cs <= 0.5 {
		return cb - (1-2*cs)*cb*(1-cb)
	}
	d := 0.0
	if cb <= 0.25 {
		d = ((16*cb-12)*cb + 4) * cb
	} else {
		d = math.Sqrt(cb)
	}
	return cb + (2*cs-1)*(d-cb)
}

func rgbToHSL(c [3]float64) (h, s, l float64) {
	max := math.Max(c[0], math.Max(c[1], c[2]))
	min := math.Min(c[0], math.Min(c[1], c[2]))
	l = (max + min) / 2
	if max == min {
		return 0, 0, l
	}
	d := max - min
	if l > 0.5 {
		s = d / (2 - max - min)
	} else {
		s = d / (max + min)
	}
	switch max {
	case c[0]:
		h = (c[1] - c[2]) / d
		if c[1] < c[2] {
			h += 6
		}
	case c[1]:
		h = (c[2]-c[0])/d + 2
	default:
		h = (c[0]-c[1])/d + 4
	}
	h /= 6
	return
}

func hslToRGB(h, s, l float64) [3]float64 {
	if s == 0 {
		return [3]float64{l, l, l}
	}
	q := l * (1 + s)
	if l >= 0.5 {
		q = l + s - l*s
	}
	p := 2*l - q
	hue := func(t float64) float64 {
		if t < 0 {
			t++
		}
		if t > 1 {
			t--
		}
		switch {
		case t < 1.0/6:
			return p + (q-p)*6*t
		case t < 1.0/2:
			return q
		case t < 2.0/3:
			return p + (q-p)*(2.0/3-t)*6
		default:
			return p
		}
	}
	return [3]float64{hue(h + 1.0/3), hue(h), hue(h - 1.0/3)}
}

func addSegments(r *vector.Rasterizer, seg []displaylist.Segment, m displaylist.Matrix, offset image.Point) {
	ox, oy := float64(offset.X), float64(offset.Y)
	p := func(pt displaylist.Point) (float32, float32) {
		q := apply(m, pt.X, pt.Y)
		return float32(q.X - ox), float32(q.Y - oy)
	}
	for _, s := range seg {
		switch s.Kind {
		case displaylist.SegmentMove:
			x, y := p(s.P0)
			r.MoveTo(x, y)
		case displaylist.SegmentLine:
			x, y := p(s.P3)
			r.LineTo(x, y)
		case displaylist.SegmentCubic:
			x1, y1 := p(s.P1)
			x2, y2 := p(s.P2)
			x3, y3 := p(s.P3)
			r.CubeTo(x1, y1, x2, y2, x3, y3)
		case displaylist.SegmentClose:
			r.ClosePath()
		}
	}
}

type grayImage struct{ image.Image }

func (g grayImage) At(x, y int) color.Color {
	r, gg, b, a := g.Image.At(x, y).RGBA()
	if a == 0 {
		return color.NRGBA{}
	}
	// RGBA() is premultiplied; undo it before computing luminance.
	r = r * 0xffff / a
	gg = gg * 0xffff / a
	b = b * 0xffff / a
	l := uint8((299*r + 587*gg + 114*b) / 1000 >> 8)
	return color.NRGBA{R: l, G: l, B: l, A: uint8(a >> 8)}
}

func (rr *regionRenderer) solidColor(rgba [4]float64, alpha float64) color.RGBA {
	if rr != nil && rr.gray {
		l := 0.299*rgba[0] + 0.587*rgba[1] + 0.114*rgba[2]
		rgba[0], rgba[1], rgba[2] = l, l, l
	}
	return solidColor(rgba, alpha)
}

func solidColor(rgba [4]float64, alpha float64) color.RGBA {
	a := clamp01(rgba[3] * alpha)
	return color.RGBA{
		R: uint8(clamp01(rgba[0]) * 255 * a),
		G: uint8(clamp01(rgba[1]) * 255 * a),
		B: uint8(clamp01(rgba[2]) * 255 * a),
		A: uint8(a * 255),
	}
}

func (rr *regionRenderer) functionShading(s *displaylist.Shading) error {
	g := s.Grid
	if g == nil || g.Width < 2 || g.Height < 2 || len(g.Colors) < g.Width*g.Height {
		return nil
	}
	inv, ok := invert(s.Matrix)
	if !ok {
		return nil
	}
	bbox := pxRect(s.Bounds, rr.pageToPx).Intersect(rr.out.Bounds())
	if bbox.Empty() {
		return nil
	}
	clip := rr.clipMask(s.Clip)
	for y := bbox.Min.Y; y < bbox.Max.Y; y++ {
		for x := bbox.Min.X; x < bbox.Max.X; x++ {
			page := displaylist.Point{X: rr.box.X0 + (float64(x)+0.5)/rr.scale, Y: rr.box.Y0 + (float64(y)+0.5)/rr.scale}
			sp := apply(inv, page.X, page.Y)
			u := (sp.X - g.Domain[0]) / (g.Domain[1] - g.Domain[0])
			v := (sp.Y - g.Domain[2]) / (g.Domain[3] - g.Domain[2])
			if u < 0 || u > 1 || v < 0 || v > 1 {
				continue
			}
			gx, gy := u*float64(g.Width-1), v*float64(g.Height-1)
			x0, y0 := int(math.Floor(gx)), int(math.Floor(gy))
			fx, fy := gx-float64(x0), gy-float64(y0)
			if x0 >= g.Width-1 {
				x0, fx = g.Width-2, 1
			}
			if y0 >= g.Height-1 {
				y0, fy = g.Height-2, 1
			}
			var c [4]float64
			for i := 0; i < 4; i++ {
				c00 := g.Colors[y0*g.Width+x0][i]
				c10 := g.Colors[y0*g.Width+x0+1][i]
				c01 := g.Colors[(y0+1)*g.Width+x0][i]
				c11 := g.Colors[(y0+1)*g.Width+x0+1][i]
				c[i] = (1-fx)*(1-fy)*c00 + fx*(1-fy)*c10 + (1-fx)*fy*c01 + fx*fy*c11
			}
			alpha := s.Alpha
			if clip != nil {
				alpha *= float64(clip.AlphaAt(x, y).A) / 255
			}
			if rr.mask != nil {
				alpha *= float64(rr.mask.AlphaAt(x, y).A) / 255
			}
			if rr.softMask != nil {
				alpha *= float64(rr.softMask.AlphaAt(x, y).A) / 255
			}
			if alpha <= 0 {
				continue
			}
			r := uint32(clamp01(c[0]) * alpha * 65535)
			gg := uint32(clamp01(c[1]) * alpha * 65535)
			b := uint32(clamp01(c[2]) * alpha * 65535)
			a := uint32(clamp01(c[3]) * alpha * 65535)
			rr.blend(x, y, r, gg, b, a, s.Blend)
		}
	}
	return nil
}

func (rr *regionRenderer) mesh(s *displaylist.Shading) error {
	if s.MeshType == 6 || s.MeshType == 7 {
		return rr.patchMesh(s)
	}
	if s.MeshType != 4 && s.MeshType != 5 || s.BitsPerCoordinate <= 0 || s.BitsPerComponent <= 0 || len(s.Raw) == 0 {
		return nil
	}
	space, err := pdfcolor.Load(context.Background(), nil, syntax.Name(s.ColorSpace), nil)
	if err != nil {
		return nil
	}
	ncomp := space.Components()
	if ncomp <= 0 {
		return nil
	}
	decode := s.Decode
	need := 2 * (2 + ncomp)
	if len(decode) < need {
		decode = make([]float64, need)
		for i := 0; i < 2; i++ {
			decode[2*i], decode[2*i+1] = 0, 1
		}
		for i := 0; i < ncomp; i++ {
			decode[4+2*i], decode[5+2*i] = 0, 1
		}
	}
	br := &bitReader{data: s.Raw}
	maxc := float64(uint64(1)<<uint(s.BitsPerCoordinate) - 1)
	maxg := float64(uint64(1)<<uint(s.BitsPerComponent) - 1)
	if s.BitsPerCoordinate == 32 {
		maxc = 4294967295
	}
	if s.BitsPerComponent == 32 {
		maxg = 4294967295
	}
	type vertex struct {
		p displaylist.Point
		c [4]float64
	}
	readVertex := func() (vertex, bool) {
		xr := float64(br.read(s.BitsPerCoordinate)) / maxc
		yr := float64(br.read(s.BitsPerCoordinate)) / maxc
		if br.err {
			return vertex{}, false
		}
		x := decode[0] + xr*(decode[1]-decode[0])
		y := decode[2] + yr*(decode[3]-decode[2])
		q := apply(mul(rr.pageToPx, s.Matrix), x, y)
		comps := make([]float64, ncomp)
		for j := 0; j < ncomp; j++ {
			raw := float64(br.read(s.BitsPerComponent)) / maxg
			comps[j] = decode[4+2*j] + raw*(decode[5+2*j]-decode[4+2*j])
		}
		rgba, _, err := space.RGBA(comps)
		if err != nil {
			return vertex{}, false
		}
		return vertex{p: q, c: rgba}, true
	}
	var verts []vertex
	var tri []int
	switch s.MeshType {
	case 5:
		if s.VerticesPerRow < 2 {
			return nil
		}
		for br.bit+s.BitsPerCoordinate*2+s.BitsPerComponent*ncomp <= len(br.data)*8 {
			v, ok := readVertex()
			if !ok {
				return nil
			}
			verts = append(verts, v)
		}
		if len(verts)%s.VerticesPerRow != 0 || len(verts)/s.VerticesPerRow < 2 {
			return nil
		}
		w, h := s.VerticesPerRow, len(verts)/s.VerticesPerRow
		for y := 0; y < h-1; y++ {
			for x := 0; x < w-1; x++ {
				i00 := y*w + x
				tri = append(tri, i00, i00+1, i00+w, i00+1, i00+w+1, i00+w)
			}
		}
	case 4:
		if s.BitsPerFlag <= 0 {
			return nil
		}
		left := 0
		for br.bit+s.BitsPerFlag+s.BitsPerCoordinate*2+s.BitsPerComponent*ncomp <= len(br.data)*8 {
			f := int(br.read(s.BitsPerFlag))
			if br.err {
				return nil
			}
			v, ok := readVertex()
			if !ok {
				return nil
			}
			if left == 0 {
				switch f {
				case 0:
					left = 3
				case 1:
					if len(tri) < 2 {
						return nil
					}
					tri = append(tri, tri[len(tri)-2], tri[len(tri)-1])
					left = 1
				case 2:
					if len(tri) < 3 {
						return nil
					}
					tri = append(tri, tri[len(tri)-3], tri[len(tri)-1])
					left = 1
				default:
					return nil
				}
			}
			tri = append(tri, len(verts))
			verts = append(verts, v)
			left--
		}
		if len(tri)%3 != 0 {
			return nil
		}
	}
	clip := rr.clipMask(s.Clip)
	for i := 0; i+2 < len(tri); i += 3 {
		a, b, c := verts[tri[i]], verts[tri[i+1]], verts[tri[i+2]]
		rr.meshTri(a.p, b.p, c.p, a.c, b.c, c.c, s, clip)
	}
	return nil
}

func (rr *regionRenderer) meshTri(a, b, c displaylist.Point, ca, cb, cc [4]float64, s *displaylist.Shading, clip *image.Alpha) {
	minX := int(math.Floor(math.Min(a.X, math.Min(b.X, c.X))))
	minY := int(math.Floor(math.Min(a.Y, math.Min(b.Y, c.Y))))
	maxX := int(math.Ceil(math.Max(a.X, math.Max(b.X, c.X))))
	maxY := int(math.Ceil(math.Max(a.Y, math.Max(b.Y, c.Y))))
	minX, minY = maxInt(minX, rr.out.Bounds().Min.X), maxInt(minY, rr.out.Bounds().Min.Y)
	maxX, maxY = minInt(maxX, rr.out.Bounds().Max.X), minInt(maxY, rr.out.Bounds().Max.Y)
	den := (b.Y-c.Y)*(a.X-c.X) + (c.X-b.X)*(a.Y-c.Y)
	if math.Abs(den) < 1e-12 {
		return
	}
	for y := minY; y < maxY; y++ {
		for x := minX; x < maxX; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			w0 := ((b.Y-c.Y)*(px-c.X) + (c.X-b.X)*(py-c.Y)) / den
			w1 := ((c.Y-a.Y)*(px-c.X) + (a.X-c.X)*(py-c.Y)) / den
			w2 := 1 - w0 - w1
			if w0 < 0 || w1 < 0 || w2 < 0 {
				continue
			}
			alpha := s.Alpha
			if clip != nil {
				alpha *= float64(clip.AlphaAt(x, y).A) / 255
			}
			if rr.mask != nil {
				alpha *= float64(rr.mask.AlphaAt(x, y).A) / 255
			}
			if rr.softMask != nil {
				alpha *= float64(rr.softMask.AlphaAt(x, y).A) / 255
			}
			if alpha <= 0 {
				continue
			}
			var c [4]float64
			for i := 0; i < 4; i++ {
				c[i] = w0*ca[i] + w1*cb[i] + w2*cc[i]
			}
			r := uint32(clamp01(c[0]) * alpha * 65535)
			g := uint32(clamp01(c[1]) * alpha * 65535)
			bl := uint32(clamp01(c[2]) * alpha * 65535)
			a := uint32(clamp01(c[3]) * alpha * 65535)
			rr.blend(x, y, r, g, bl, a, s.Blend)
		}
	}
}

type bitReader struct {
	data []byte
	bit  int
	err  bool
}

func (r *bitReader) read(n int) uint32 {
	if n <= 0 || n > 32 {
		r.err = true
		return 0
	}
	var v uint32
	for i := 0; i < n; i++ {
		pos := r.bit + i
		if pos/8 >= len(r.data) {
			r.err = true
			return v
		}
		v = v<<1 | uint32((r.data[pos/8]>>uint(7-pos%8))&1)
	}
	r.bit += n
	return v
}

func shadingColor(stops []displaylist.ColorStop, t float64) color.RGBA {
	if t <= stops[0].Offset {
		c := stops[0].Color
		return color.RGBA{uint8(clamp01(c[0]) * 255), uint8(clamp01(c[1]) * 255), uint8(clamp01(c[2]) * 255), uint8(clamp01(c[3]) * 255)}
	}
	for i := 1; i < len(stops); i++ {
		if t <= stops[i].Offset {
			a, b := stops[i-1], stops[i]
			f := 0.0
			if b.Offset > a.Offset {
				f = (t - a.Offset) / (b.Offset - a.Offset)
			}
			return color.RGBA{
				uint8(clamp01(a.Color[0]+(b.Color[0]-a.Color[0])*f) * 255),
				uint8(clamp01(a.Color[1]+(b.Color[1]-a.Color[1])*f) * 255),
				uint8(clamp01(a.Color[2]+(b.Color[2]-a.Color[2])*f) * 255),
				uint8(clamp01(a.Color[3]+(b.Color[3]-a.Color[3])*f) * 255),
			}
		}
	}
	c := stops[len(stops)-1].Color
	return color.RGBA{uint8(clamp01(c[0]) * 255), uint8(clamp01(c[1]) * 255), uint8(clamp01(c[2]) * 255), uint8(clamp01(c[3]) * 255)}
}

func pxRect(r displaylist.Rect, m displaylist.Matrix) image.Rectangle {
	p0 := apply(m, r.X0, r.Y0)
	p1 := apply(m, r.X1, r.Y1)
	return image.Rect(int(math.Floor(math.Min(p0.X, p1.X))), int(math.Floor(math.Min(p0.Y, p1.Y))), int(math.Ceil(math.Max(p0.X, p1.X))), int(math.Ceil(math.Max(p0.Y, p1.Y))))
}

func imageRect(m, p2p displaylist.Matrix) image.Rectangle {
	corners := []displaylist.Point{
		apply(mul(p2p, m), 0, 0), apply(mul(p2p, m), 1, 0),
		apply(mul(p2p, m), 1, 1), apply(mul(p2p, m), 0, 1),
	}
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, p := range corners {
		minX, minY = math.Min(minX, p.X), math.Min(minY, p.Y)
		maxX, maxY = math.Max(maxX, p.X), math.Max(maxY, p.Y)
	}
	return image.Rect(int(math.Floor(minX)), int(math.Floor(minY)), int(math.Ceil(maxX)), int(math.Ceil(maxY)))
}

func apply(m displaylist.Matrix, x, y float64) displaylist.Point {
	return displaylist.Point{X: m[0]*x + m[2]*y + m[4], Y: m[1]*x + m[3]*y + m[5]}
}

func mul(a, b displaylist.Matrix) displaylist.Matrix {
	return displaylist.Matrix{
		a[0]*b[0] + a[2]*b[1], a[1]*b[0] + a[3]*b[1],
		a[0]*b[2] + a[2]*b[3], a[1]*b[2] + a[3]*b[3],
		a[0]*b[4] + a[2]*b[5] + a[4], a[1]*b[4] + a[3]*b[5] + a[5],
	}
}

func invert(m displaylist.Matrix) (displaylist.Matrix, bool) {
	det := m[0]*m[3] - m[1]*m[2]
	if math.Abs(det) < 1e-12 {
		return displaylist.Matrix{}, false
	}
	id := 1 / det
	return displaylist.Matrix{
		m[3] * id, -m[1] * id, -m[2] * id, m[0] * id,
		(m[2]*m[5] - m[3]*m[4]) * id, (m[1]*m[4] - m[0]*m[5]) * id,
	}, true
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func strokePoints(r *vector.Rasterizer, pts []displaylist.Point, half float64, line displaylist.LineStyle, closed bool) {
	if len(pts) < 2 {
		return
	}
	if closed && len(pts) > 2 && pts[0] == pts[len(pts)-1] {
		pts = pts[:len(pts)-1]
	}
	if len(line.Dash) == 0 {
		strokeRun(r, pts, half, line.Cap, line.Join, line.Miter, closed)
		return
	}
	pattern := append([]float64(nil), line.Dash...)
	if len(pattern)%2 == 1 {
		pattern = append(pattern, pattern...)
	}
	period := 0.0
	for _, d := range pattern {
		if d <= 0 {
			return
		}
		period += d
	}
	pos := math.Mod(line.Phase, period)
	if pos < 0 {
		pos += period
	}
	idx := 0
	for pos >= pattern[idx] {
		pos -= pattern[idx]
		idx = (idx + 1) % len(pattern)
	}
	on := idx%2 == 0
	remaining := pattern[idx] - pos
	var run []displaylist.Point
	if on {
		run = append(run, pts[0])
	}
	for i := 0; i+1 < len(pts); i++ {
		a, b := pts[i], pts[i+1]
		dx, dy := b.X-a.X, b.Y-a.Y
		length := math.Hypot(dx, dy)
		if length == 0 {
			continue
		}
		ux, uy := dx/length, dy/length
		for length > 1e-9 {
			step := math.Min(length, remaining)
			next := displaylist.Point{X: a.X + ux*step, Y: a.Y + uy*step}
			if on {
				run = append(run, next)
			}
			a = next
			length -= step
			remaining -= step
			if remaining <= 1e-9 {
				if on && len(run) >= 2 {
					strokeRun(r, run, half, line.Cap, line.Join, line.Miter, false)
				}
				on = !on
				idx = (idx + 1) % len(pattern)
				remaining = pattern[idx]
				if on {
					run = []displaylist.Point{a}
				} else {
					run = nil
				}
			}
		}
	}
	if on && len(run) >= 2 {
		strokeRun(r, run, half, line.Cap, line.Join, line.Miter, false)
	}
}

func strokeRun(r *vector.Rasterizer, pts []displaylist.Point, half float64, cap, join int, miterLimit float64, closed bool) {
	if len(pts) < 2 {
		return
	}
	for i := 0; i+1 < len(pts); i++ {
		addStrokeSegment(r, pts[i], pts[i+1], half)
	}
	for i := 1; i+1 < len(pts); i++ {
		addJoin(r, pts[i-1], pts[i], pts[i+1], half, join, miterLimit)
	}
	if closed {
		addStrokeSegment(r, pts[len(pts)-1], pts[0], half)
		addJoin(r, pts[len(pts)-2], pts[len(pts)-1], pts[0], half, join, miterLimit)
		addJoin(r, pts[len(pts)-1], pts[0], pts[1], half, join, miterLimit)
		return
	}
	addCap(r, pts[0], pts[1], half, cap)
	addCap(r, pts[len(pts)-1], pts[len(pts)-2], half, cap)
}

func addJoin(r *vector.Rasterizer, prev, cur, next displaylist.Point, half float64, join int, miterLimit float64) {
	if join == 1 {
		addDisc(r, cur, half)
		return
	}
	d1 := displaylist.Point{X: cur.X - prev.X, Y: cur.Y - prev.Y}
	d2 := displaylist.Point{X: next.X - cur.X, Y: next.Y - cur.Y}
	l1, l2 := math.Hypot(d1.X, d1.Y), math.Hypot(d2.X, d2.Y)
	if l1 == 0 || l2 == 0 {
		return
	}
	d1.X, d1.Y, d2.X, d2.Y = d1.X/l1, d1.Y/l1, d2.X/l2, d2.Y/l2
	cross := d1.X*d2.Y - d1.Y*d2.X
	if math.Abs(cross) < 1e-9 {
		return
	}
	side := 1.0
	if cross > 0 {
		side = -1
	}
	a := displaylist.Point{X: cur.X - d1.Y*side*half, Y: cur.Y + d1.X*side*half}
	b := displaylist.Point{X: cur.X - d2.Y*side*half, Y: cur.Y + d2.X*side*half}
	if join == 2 || miterLimit <= 0 {
		addPolygon(r, []displaylist.Point{cur, a, b})
		return
	}
	// Intersect the two offset lines for the miter point.
	dx, dy := b.X-a.X, b.Y-a.Y
	den := d1.X*d2.Y - d1.Y*d2.X
	if math.Abs(den) < 1e-12 {
		addPolygon(r, []displaylist.Point{cur, a, b})
		return
	}
	t := (dx*d2.Y - dy*d2.X) / den
	m := displaylist.Point{X: a.X + d1.X*t, Y: a.Y + d1.Y*t}
	if math.Hypot(m.X-cur.X, m.Y-cur.Y) > miterLimit*half {
		addPolygon(r, []displaylist.Point{cur, a, b})
		return
	}
	addPolygon(r, []displaylist.Point{cur, a, m, b})
}

func addStrokeSegment(r *vector.Rasterizer, a, b displaylist.Point, half float64) {
	dx, dy := b.X-a.X, b.Y-a.Y
	l := math.Hypot(dx, dy)
	if l == 0 {
		return
	}
	nx, ny := -dy/l*half, dx/l*half
	addPolygon(r, []displaylist.Point{
		{X: a.X + nx, Y: a.Y + ny},
		{X: b.X + nx, Y: b.Y + ny},
		{X: b.X - nx, Y: b.Y - ny},
		{X: a.X - nx, Y: a.Y - ny},
	})
}

func addCap(r *vector.Rasterizer, p, toward displaylist.Point, half float64, cap int) {
	if cap == 0 {
		return
	}
	if cap == 1 {
		addDisc(r, p, half)
		return
	}
	dx, dy := p.X-toward.X, p.Y-toward.Y
	l := math.Hypot(dx, dy)
	if l == 0 {
		return
	}
	ux, uy := dx/l*half, dy/l*half
	nx, ny := -dy/l*half, dx/l*half
	addPolygon(r, []displaylist.Point{
		{X: p.X + nx, Y: p.Y + ny},
		{X: p.X + nx + ux, Y: p.Y + ny + uy},
		{X: p.X - nx + ux, Y: p.Y - ny + uy},
		{X: p.X - nx, Y: p.Y - ny},
	})
}

func addDisc(r *vector.Rasterizer, p displaylist.Point, radius float64) {
	const n = 16
	for i := 0; i <= n; i++ {
		a := 2 * math.Pi * float64(i) / n
		q := displaylist.Point{X: p.X + radius*math.Cos(a), Y: p.Y + radius*math.Sin(a)}
		if i == 0 {
			r.MoveTo(float32(q.X), float32(q.Y))
		} else {
			r.LineTo(float32(q.X), float32(q.Y))
		}
	}
	r.ClosePath()
}

func addPolygon(r *vector.Rasterizer, pts []displaylist.Point) {
	if len(pts) < 2 {
		return
	}
	r.MoveTo(float32(pts[0].X), float32(pts[0].Y))
	for _, p := range pts[1:] {
		r.LineTo(float32(p.X), float32(p.Y))
	}
	r.ClosePath()
}

func flattenPath(seg []displaylist.Segment, m displaylist.Matrix, offset image.Point) [][]displaylist.Point {
	var out [][]displaylist.Point
	var cur []displaylist.Point
	pt := func(p displaylist.Point) displaylist.Point {
		q := apply(m, p.X, p.Y)
		return displaylist.Point{X: q.X - float64(offset.X), Y: q.Y - float64(offset.Y)}
	}
	flatten := func(p0, p1, p2, p3 displaylist.Point) {
		const steps = 16
		for i := 1; i <= steps; i++ {
			t := float64(i) / steps
			u := 1 - t
			cur = append(cur, displaylist.Point{
				X: u*u*u*p0.X + 3*u*u*t*p1.X + 3*u*t*t*p2.X + t*t*t*p3.X,
				Y: u*u*u*p0.Y + 3*u*u*t*p1.Y + 3*u*t*t*p2.Y + t*t*t*p3.Y,
			})
		}
	}
	for _, s := range seg {
		switch s.Kind {
		case displaylist.SegmentMove:
			if len(cur) > 0 {
				out = append(out, cur)
			}
			cur = []displaylist.Point{pt(s.P0)}
		case displaylist.SegmentLine:
			if len(cur) == 0 {
				continue
			}
			cur = append(cur, pt(s.P3))
		case displaylist.SegmentCubic:
			if len(cur) == 0 {
				continue
			}
			flatten(cur[len(cur)-1], pt(s.P1), pt(s.P2), pt(s.P3))
		case displaylist.SegmentClose:
			if len(cur) > 0 {
				cur = append(cur, cur[0])
			}
		}
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

func fillEvenOdd(dst *image.RGBA, paths [][]displaylist.Point, c color.RGBA) {
	const samples = 4
	if len(paths) == 0 {
		return
	}
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, path := range paths {
		for _, p := range path {
			minX, minY = math.Min(minX, p.X), math.Min(minY, p.Y)
			maxX, maxY = math.Max(maxX, p.X), math.Max(maxY, p.Y)
		}
	}
	x0, y0 := int(math.Floor(minX)), int(math.Floor(minY))
	x1, y1 := int(math.Ceil(maxX)), int(math.Ceil(maxY))
	x0, y0 = maxInt(x0, dst.Bounds().Min.X), maxInt(y0, dst.Bounds().Min.Y)
	x1, y1 = minInt(x1, dst.Bounds().Max.X), minInt(y1, dst.Bounds().Max.Y)
	var xs [samples][]float64
	for y := y0; y < y1; y++ {
		for sy := 0; sy < samples; sy++ {
			py := float64(y) + (float64(sy)+0.5)/samples
			xs[sy] = xs[sy][:0]
			for _, path := range paths {
				for i, j := 0, len(path)-1; i < len(path); j, i = i, i+1 {
					pi, pj := path[i], path[j]
					if (pi.Y > py) != (pj.Y > py) {
						xs[sy] = append(xs[sy], (pj.X-pi.X)*(py-pi.Y)/(pj.Y-pi.Y)+pi.X)
					}
				}
			}
			sort.Float64s(xs[sy])
		}
		for x := x0; x < x1; x++ {
			inside := 0
			for sy := 0; sy < samples; sy++ {
				for sx := 0; sx < samples; sx++ {
					px := float64(x) + (float64(sx)+0.5)/samples
					n := len(xs[sy]) - sort.SearchFloat64s(xs[sy], px)
					// SearchFloat64s returns the first value >= px. Only
					// intersections strictly to the right toggle parity.
					for n > 0 && xs[sy][len(xs[sy])-n] == px {
						n--
					}
					if n&1 != 0 {
						inside++
					}
				}
			}
			if inside == 0 {
				continue
			}
			coverage := float64(inside) / (samples * samples)
			cc := color.RGBA{R: uint8(float64(c.R) * coverage), G: uint8(float64(c.G) * coverage), B: uint8(float64(c.B) * coverage), A: uint8(float64(c.A) * coverage)}
			drawPixel(dst, x, y, cc)
		}
	}
}

func drawPixel(dst *image.RGBA, x, y int, c color.RGBA) {
	if c.A == 0 {
		return
	}
	dr, dg, db, da := dst.At(x, y).RGBA()
	a := uint32(c.A) * 0x101
	inv := 65535 - a
	dst.SetRGBA(x, y, color.RGBA{
		R: uint8((uint32(c.R)*0x101 + dr*inv/65535) >> 8),
		G: uint8((uint32(c.G)*0x101 + dg*inv/65535) >> 8),
		B: uint8((uint32(c.B)*0x101 + db*inv/65535) >> 8),
		A: uint8((a + da*inv/65535) >> 8),
	})
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (rr *regionRenderer) softMaskFor(sm *displaylist.SoftMask) (*image.Alpha, error) {
	if sm == nil {
		return nil, nil
	}
	if m, ok := rr.softMasks[sm]; ok {
		return m, nil
	}
	maskRGBA := image.NewRGBA(rr.out.Bounds())
	if sm.HasBackdrop {
		draw.Draw(maskRGBA, maskRGBA.Bounds(), image.NewUniform(solidColor(sm.Backdrop, sm.Backdrop[3])), image.Point{}, draw.Src)
	}
	maskRelease, err := rr.acquire(int64(len(maskRGBA.Pix)))
	if err != nil {
		return nil, err
	}
	defer maskRelease()
	child := &regionRenderer{
		ctx: rr.ctx, dl: rr.dl, out: maskRGBA, box: rr.box, scale: rr.scale,
		pageToPx: rr.pageToPx, opts: rr.opts, clips: map[int]*image.Alpha{},
		clipList: sm.Clips, inMask: true, softMasks: map[*displaylist.SoftMask]*image.Alpha{},
		memory: rr.memory,
	}
	defer child.releaseAll()
	for _, it := range sm.Items {
		if rr.opts.Exclude != nil && rr.opts.Exclude(it) {
			continue
		}
		if err := child.renderItem(it); err != nil {
			return nil, err
		}
	}
	m := image.NewAlpha(rr.out.Bounds())
	release, err := rr.acquire(int64(len(m.Pix)))
	if err != nil {
		return nil, err
	}
	rr.leases = append(rr.leases, release)
	for y := 0; y < rr.out.Bounds().Dy(); y++ {
		for x := 0; x < rr.out.Bounds().Dx(); x++ {
			c := maskRGBA.RGBAAt(x, y)
			a := float64(c.A)
			if strings.EqualFold(sm.Subtype, "Luminosity") {
				a *= (0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)) / 255
			}
			m.SetAlpha(x, y, color.Alpha{A: uint8(a)})
		}
	}
	rr.softMasks[sm] = m
	return m, nil
}

func annotationVisible(a displaylist.Annotation, mode AnnotationMode) bool {
	if mode != AnnotationView {
		return false
	}
	const (
		flagInvisible = 1
		flagHidden    = 2
		flagNoView    = 32
	)
	return a.Flags&(flagInvisible|flagHidden|flagNoView) == 0
}
