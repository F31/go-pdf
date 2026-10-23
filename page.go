package pdf

import (
	"context"
	"math"

	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/internal/doc"
	"github.com/F31/go-pdf/internal/errs"
	pdftext "github.com/F31/go-pdf/text"
)

// Box is an axis-aligned rectangle in PDF user space.
type Box struct {
	X0, Y0, X1, Y1 float64
}

type PageInfo struct {
	Index    int
	Width    float64
	Height   float64
	Rotate   int
	UserUnit float64
	MediaBox Box
	CropBox  Box
	Matrix   [6]float64
}

// Page is a single page handle. It is not safe for concurrent use.
type Page struct {
	inner *doc.Page
	doc   *Document
}

func (p *Page) Index() int {
	if p == nil || p.inner == nil {
		return -1
	}
	return p.inner.Index()
}

func (p *Page) Size() (w, h float64) {
	if p == nil || p.inner == nil {
		return 0, 0
	}
	return p.inner.Size()
}

func (p *Page) Info() PageInfo {
	if p == nil || p.inner == nil {
		return PageInfo{}
	}
	i := p.inner.Info()
	return PageInfo{
		Index: i.Index, Width: i.Width, Height: i.Height, Rotate: i.Rotate,
		UserUnit: i.UserUnit, Matrix: i.Matrix,
		MediaBox: Box{X0: i.MediaBox.X0, Y0: i.MediaBox.Y0, X1: i.MediaBox.X1, Y1: i.MediaBox.Y1},
		CropBox:  Box{X0: i.CropBox.X0, Y0: i.CropBox.Y0, X1: i.CropBox.X1, Y1: i.CropBox.Y1},
	}
}

// Walk streams display items to fn in paint order.
func (p *Page) Walk(ctx context.Context, fn func(displaylist.Item) error) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap(errs.ErrCanceled, "CNT-CANCELED", "walk page", -1, 0, p.Index())
	}
	if p == nil || p.inner == nil {
		return errs.Wrap(errs.ErrClosed, "DOC-CLOSED", "walk page", -1, 0, -1)
	}
	_, err := p.inner.Interpret(ctx, fn)
	return err
}

// Annotations returns generic annotation and widget metadata.
func (p *Page) Annotations(ctx context.Context) ([]displaylist.Annotation, error) {
	if p == nil || p.inner == nil {
		return nil, errs.Wrap(errs.ErrClosed, "DOC-CLOSED", "list annotations", -1, 0, -1)
	}
	return p.inner.Annotations(ctx)
}

// Links returns link annotations with page-space rectangles.
func (p *Page) Links(ctx context.Context) ([]displaylist.Link, error) {
	if p == nil || p.inner == nil {
		return nil, errs.Wrap(errs.ErrClosed, "DOC-CLOSED", "list links", -1, 0, -1)
	}
	return p.inner.Links(ctx)
}

// Label returns the page label (for example "iv" or "A-3").
func (p *Page) Label(ctx context.Context) (string, error) {
	if p == nil || p.inner == nil {
		return "", errs.Wrap(errs.ErrClosed, "DOC-CLOSED", "page label", -1, 0, -1)
	}
	return p.inner.Document().PageLabel(ctx, p.Index()), nil
}

// Text extracts a basic character/word/line index from the page.
func (p *Page) Text(ctx context.Context) (*pdftext.Page, error) {
	dl, err := p.DisplayList(ctx)
	if err != nil {
		return nil, err
	}
	return pdftext.Extract(dl, pdftext.Options{}), nil
}

// Search finds query occurrences in the page text.
func (p *Page) Search(ctx context.Context, query string, opts pdftext.SearchOptions) ([]pdftext.Hit, error) {
	pg, err := p.Text(ctx)
	if err != nil {
		return nil, err
	}
	return pg.Search(query, opts), nil
}

// AcquireMemory reserves live memory from the owning document for a transient
// raster buffer. The returned release function is idempotent.
func (p *Page) AcquireMemory(bytes int64) (func(), error) {
	if p == nil || p.doc == nil || p.doc.tracker == nil {
		return nil, errs.Wrap(errs.ErrClosed, "DOC-CLOSED", "acquire page memory", -1, 0, -1)
	}
	return p.doc.tracker.Acquire(bytes)
}

// DisplayList materializes the page display list.
func (p *Page) DisplayList(ctx context.Context) (*displaylist.Page, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap(errs.ErrCanceled, "CNT-CANCELED", "display list", -1, 0, p.Index())
	}
	if p == nil || p.inner == nil {
		return nil, errs.Wrap(errs.ErrClosed, "DOC-CLOSED", "display list", -1, 0, -1)
	}
	items := make([]displaylist.Item, 0, 16)
	res, err := p.inner.Interpret(ctx, func(it displaylist.Item) error {
		items = append(items, it)
		return nil
	})
	if err != nil {
		return nil, err
	}
	info := p.Info()
	diags := make([]displaylist.Diagnostic, 0)
	for _, d := range p.doc.inner.DiagnosticsForPage(p.Index()) {
		if d.Page == p.Index() {
			diags = append(diags, displaylist.Diagnostic{
				Level: d.Level.String(), Code: d.Code, Msg: d.Msg,
				Page: d.Page, Obj: d.Obj, Offset: d.Offset,
			})
		}
	}
	label, _ := p.Label(ctx)
	page := &displaylist.Page{
		Index: info.Index, Width: info.Width, Height: info.Height, Rotate: info.Rotate, Label: label,
		MediaBox: rect(info.MediaBox), CropBox: rect(info.CropBox),
		Clips: res.Clips, Items: items, Diag: diags,
	}
	page.Links, _ = p.inner.Links(ctx)
	page.Annotations, _ = p.inner.Annotations(ctx)
	if tree, err := p.doc.StructureTree(ctx); err == nil {
		ids := structureIDs(tree, page.Index)
		walkItems(page.Items, func(it displaylist.Item) {
			c := it.Base()
			if c.MCID >= 0 {
				if id, ok := ids[c.MCID]; ok {
					c.StructRef = id
				}
			}
		})
	}
	page.Scanned = scannedPage(items, page.CropBox)
	return page, nil
}

func rect(b Box) displaylist.Rect {
	return displaylist.Rect{X0: b.X0, Y0: b.Y0, X1: b.X1, Y1: b.Y1}
}

func (p *Page) Images(ctx context.Context) ([]displaylist.ImageInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap(errs.ErrCanceled, "IMG-CANCELED", "list images", -1, 0, p.Index())
	}
	dl, err := p.DisplayList(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]displaylist.ImageInfo, 0, 4)
	seen := map[displaylist.ImageRef]bool{}
	walkItems(dl.Items, func(it displaylist.Item) {
		im, ok := it.(*displaylist.Image)
		if !ok || im.Data == nil || seen[im.Data] {
			return
		}
		seen[im.Data] = true
		out = append(out, im.Info)
	})
	return out, nil
}

func (p *Page) Fonts(ctx context.Context) ([]*displaylist.FontInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap(errs.ErrCanceled, "FNT-CANCELED", "list fonts", -1, 0, p.Index())
	}
	dl, err := p.DisplayList(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*displaylist.FontInfo, 0, 4)
	seen := map[*displaylist.FontInfo]bool{}
	walkItems(dl.Items, func(it displaylist.Item) {
		tr, ok := it.(*displaylist.TextRun)
		if !ok || tr.Font == nil || seen[tr.Font] {
			return
		}
		seen[tr.Font] = true
		out = append(out, tr.Font)
	})
	return out, nil
}

func scannedPage(items []displaylist.Item, crop displaylist.Rect) bool {
	cropArea := (crop.X1 - crop.X0) * (crop.Y1 - crop.Y0)
	if cropArea <= 0 {
		return false
	}
	hasText := false
	hasImage := false
	walkItems(items, func(it displaylist.Item) {
		switch v := it.(type) {
		case *displaylist.TextRun:
			for _, g := range v.Glyphs {
				if g.UnicodeOK {
					hasText = true
					break
				}
			}
		case *displaylist.Image:
			b := v.Bounds
			x0, y0 := math.Max(b.X0, crop.X0), math.Max(b.Y0, crop.Y0)
			x1, y1 := math.Min(b.X1, crop.X1), math.Min(b.Y1, crop.Y1)
			if x1 > x0 && y1 > y0 && (x1-x0)*(y1-y0) >= cropArea*0.9 {
				hasImage = true
			}
		}
	})
	return hasImage && !hasText
}

func walkItems(items []displaylist.Item, fn func(displaylist.Item)) {
	for _, it := range items {
		fn(it)
		if g, ok := it.(*displaylist.Group); ok {
			walkItems(g.Items, fn)
		}
	}
}

func structureIDs(tree []StructElement, page int) map[int]int {
	ids := map[int]int{}
	var walk func([]StructElement)
	walk = func(nodes []StructElement) {
		for _, n := range nodes {
			if n.Page == page {
				for _, mcid := range n.MCIDs {
					if n.ID > 0 {
						ids[mcid] = n.ID
					}
				}
			}
			walk(n.Children)
		}
	}
	walk(tree)
	return ids
}
