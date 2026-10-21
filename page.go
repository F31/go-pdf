package pdf

import (
	"context"

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

// Walk streams display items to fn. G0 does not implement content
// interpretation and returns ErrUnsupported with a diagnostic.
func (p *Page) Walk(ctx context.Context, fn func(displaylist.Item) error) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap(errs.ErrCanceled, "CNT-CANCELED", "walk page", -1, 0, p.Index())
	}
	return p.unsupported("walk display list")
}

// DisplayList materializes the display list. G0 returns ErrUnsupported.
func (p *Page) DisplayList(ctx context.Context) (*displaylist.Page, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap(errs.ErrCanceled, "CNT-CANCELED", "display list", -1, 0, p.Index())
	}
	return nil, p.unsupported("display list")
}

func (p *Page) Images(ctx context.Context) ([]displaylist.ImageInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap(errs.ErrCanceled, "IMG-CANCELED", "list images", -1, 0, p.Index())
	}
	return nil, p.unsupported("list images")
}

func (p *Page) Fonts(ctx context.Context) ([]*displaylist.FontInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap(errs.ErrCanceled, "FNT-CANCELED", "list fonts", -1, 0, p.Index())
	}
	return nil, p.unsupported("list fonts")
}

func (p *Page) unsupported(op string) error {
	if p == nil || p.doc == nil || p.doc.inner == nil {
		return errs.Wrap(errs.ErrClosed, "DOC-CLOSED", op, -1, 0, -1)
	}
	return p.doc.inner.Unsupported("CNT-NOT-IMPLEMENTED", op, p.Index())
}
