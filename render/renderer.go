package render

import (
	"context"
	"io"
	"strings"

	"github.com/F31/go-pdf"
	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/extension"
	"github.com/F31/go-pdf/internal/errs"
)

// RenderProfile captures the default quality and feature policy for a renderer.
type RenderProfile struct {
	Name        string
	DPI         int
	IncludeText bool
	Annotations AnnotationMode
}

// ProfilePreview prioritizes latency for interactive previews and enables the
// built-in text fallback when no font provider is configured.
var ProfilePreview = RenderProfile{Name: "preview", DPI: 120, IncludeText: true, Annotations: AnnotationView}

// Options configures a Renderer. Zero values use ProfilePreview.
type Options struct {
	Profile    RenderProfile
	Fonts      FontProvider
	FontEngine extension.FontEngine
	Memory     MemoryBudget
}

// Renderer applies a stable profile and shared providers to page rendering.
type Renderer struct {
	opts Options
}

// NewRenderer validates options and returns a reusable renderer.
func NewRenderer(opts Options) (*Renderer, error) {
	if opts.Profile.Name == "" {
		opts.Profile = ProfilePreview
	}
	if opts.Profile.DPI < 0 {
		return nil, errs.Wrap(errs.ErrCorrupt, "RND-PROFILE", "invalid render profile DPI", -1, 0, -1)
	}
	if opts.Fonts == nil && opts.Profile.IncludeText {
		opts.Fonts = defaultTextFonts()
	}
	return &Renderer{opts: opts}, nil
}

func (r *Renderer) apply(opts PageOptions) PageOptions {
	if opts.DPI <= 0 {
		opts.DPI = r.opts.Profile.DPI
	}
	if !opts.IncludeText {
		opts.IncludeText = r.opts.Profile.IncludeText
	}
	if opts.Annotations == AnnotationNone {
		opts.Annotations = r.opts.Profile.Annotations
	}
	if opts.Fonts == nil {
		opts.Fonts = r.opts.Fonts
	}
	if opts.FontEngine == nil {
		opts.FontEngine = r.opts.FontEngine
	}
	if opts.Memory == nil {
		opts.Memory = r.opts.Memory
	}
	return opts
}

// RenderRegion renders a page-space rectangle using the renderer profile.
func (r *Renderer) RenderRegion(ctx context.Context, page *pdf.Page, box displaylist.Rect, opts PageOptions) (*Raster, error) {
	return RenderRegionLease(ctx, page, box, r.apply(opts))
}

// RenderPagesEach renders pages with the renderer profile and streams them to
// fn. See RenderPagesEach for callback and ownership rules.
func (r *Renderer) RenderPagesEach(ctx context.Context, doc *pdf.Document, pages []int, opts PageOptions, workers int, fn func(PageResult) error) error {
	return RenderPagesEach(ctx, doc, pages, r.apply(opts), workers, fn)
}

// RenderedPage is an owned page raster with its effective capabilities.
type RenderedPage struct {
	*Raster
	Capabilities RenderCapabilities
}

// RenderImage renders the page CropBox using the renderer profile.
func (r *Renderer) RenderImage(ctx context.Context, page *pdf.Page, opts PageOptions) (*Raster, error) {
	return RenderPageLease(ctx, page, r.apply(opts))
}

// RenderPage renders the page CropBox and reports capabilities.
func (r *Renderer) RenderPage(ctx context.Context, page *pdf.Page, opts PageOptions) (*RenderedPage, error) {
	effective := r.apply(opts)
	raster, err := RenderPageLease(ctx, page, effective)
	if err != nil {
		return nil, err
	}
	caps, err := r.Capabilities(ctx, page)
	if err != nil {
		raster.Release()
		return nil, err
	}
	caps.EffectiveProfile = r.opts.Profile.Name
	caps.EffectiveDPI = effective.DPI
	return &RenderedPage{Raster: raster, Capabilities: caps}, nil
}

// EncodeTo renders the page and encodes it as PNG or JPEG.
func (r *Renderer) EncodeTo(ctx context.Context, w io.Writer, page *pdf.Page, opts PageOptions, format string, quality int) error {
	raster, err := r.RenderImage(ctx, page, opts)
	if err != nil {
		return err
	}
	defer raster.Release()
	switch strings.ToLower(format) {
	case "png":
		return EncodePNG(w, raster.Image)
	case "jpg", "jpeg":
		return EncodeJPEG(w, raster.Image, quality)
	default:
		return errs.Wrap(errs.ErrUnsupported, "RND-FORMAT", "unsupported render output format", -1, 0, -1)
	}
}

// RenderableStatus summarizes whether a page can be rendered faithfully.
type RenderableStatus uint8

const (
	RenderableFull RenderableStatus = iota
	RenderableApproximate
	RenderableNotScanned
	RenderableUnsupported
)

// CapabilityIssue records one known limitation found while scanning a page.
type CapabilityIssue struct {
	Code    string
	Count   int
	Message string
}

// RenderCapabilities is the render-time status for a display list.
type RenderCapabilities struct {
	Status              RenderableStatus
	Items               int
	ApproximateItems    int
	MissingTextOutlines int
	ExternalImages      int
	EffectiveProfile    string
	EffectiveDPI        int
	Notes               []string
	Issues              []CapabilityIssue
}

// Capabilities scans a page display list for known render limitations.
func (r *Renderer) Capabilities(ctx context.Context, page *pdf.Page) (RenderCapabilities, error) {
	if page == nil {
		return RenderCapabilities{}, errs.Wrap(errs.ErrClosed, "RND-PAGE", "render capabilities", -1, 0, -1)
	}
	dl, err := page.DisplayList(ctx)
	if err != nil {
		return RenderCapabilities{}, err
	}
	caps := RenderCapabilities{EffectiveProfile: r.opts.Profile.Name}
	var walk func([]displaylist.Item)
	walk = func(items []displaylist.Item) {
		for _, it := range items {
			caps.Items++
			if it.Base().Flags&displaylist.FlagApprox != 0 {
				caps.ApproximateItems++
			}
			switch v := it.(type) {
			case *displaylist.TextRun:
				if r.opts.Fonts == nil && !hasEngineSource(v, r.opts.FontEngine) && !hasTextOutlines(v) {
					caps.MissingTextOutlines++
				}
			case *displaylist.Group:
				walk(v.Items)
			case *displaylist.Image:
				switch strings.ToLower(string(v.Info.Encoding)) {
				case "jbig2decode", "jpxdecode":
					caps.ExternalImages++
				}
			}
		}
	}
	walk(dl.Items)
	switch {
	case caps.MissingTextOutlines > 0:
		caps.Status = RenderableApproximate
		caps.Notes = append(caps.Notes, "text runs without available outlines are skipped")
		caps.Issues = append(caps.Issues, CapabilityIssue{Code: "RND-CAP-TEXT-NO-OUTLINE", Count: caps.MissingTextOutlines, Message: "text runs without available outlines are skipped"})
	case caps.ApproximateItems > 0 || caps.ExternalImages > 0:
		if caps.ExternalImages > 0 {
			caps.Status = RenderableNotScanned
		} else {
			caps.Status = RenderableApproximate
		}
		if caps.ApproximateItems > 0 {
			caps.Notes = append(caps.Notes, "display list contains approximate items")
			caps.Issues = append(caps.Issues, CapabilityIssue{Code: "RND-CAP-APPROX", Count: caps.ApproximateItems, Message: "display list contains approximate items"})
		}
		if caps.ExternalImages > 0 {
			caps.Notes = append(caps.Notes, "some images require an external decoder")
			caps.Issues = append(caps.Issues, CapabilityIssue{Code: "RND-CAP-EXTERNAL-DECODER", Count: caps.ExternalImages, Message: "images require an external decoder"})
		}
	default:
		caps.Status = RenderableFull
	}
	return caps, nil
}

func hasTextOutlines(t *displaylist.TextRun) bool {
	if t == nil || t.Font == nil {
		return false
	}
	if t.Font.Kind == "Type3" {
		return true
	}
	if t.Font.GlyphSource == nil {
		return false
	}
	for _, g := range t.Glyphs {
		if _, ok := t.Font.GlyphSource.GlyphOutline(g.Code, g.CID); ok {
			return true
		}
	}
	return false
}

func hasEngineSource(t *displaylist.TextRun, engine extension.FontEngine) bool {
	return engine != nil && t != nil && t.Font != nil && t.Font.Source != nil
}
