package pdf

import (
	"context"
	"math"

	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/internal/doc"
	"github.com/F31/go-pdf/internal/errs"
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
	for _, d := range p.doc.Diagnostics() {
		if d.Page == p.Index() {
			diags = append(diags, displaylist.Diagnostic{
				Level: d.Level.String(), Code: d.Code, Msg: d.Msg,
				Page: d.Page, Obj: d.Obj, Offset: d.Offset,
			})
		}
	}
	page := &displaylist.Page{
		Index: info.Index, Width: info.Width, Height: info.Height, Rotate: info.Rotate,
		MediaBox: rect(info.MediaBox), CropBox: rect(info.CropBox),
		Clips: res.Clips, Items: items, Diag: diags,
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
