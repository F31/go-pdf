package render

import (
	"context"
	stdimage "image"
	"image/jpeg"
	"image/png"
	"io"
	"sync"

	"github.com/F31/go-pdf"
	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/internal/errs"
)

// Raster is an owned rendered image with a live-memory lease.
type Raster struct {
	Image   stdimage.Image
	release func()
	once    sync.Once
}

// Release returns the raster's memory lease. It is idempotent.
func (r *Raster) Release() {
	if r == nil {
		return
	}
	r.once.Do(func() {
		if r.release != nil {
			r.release()
		}
	})
}

// Close is an alias for Release.
func (r *Raster) Close() error {
	r.Release()
	return nil
}

// RenderPageLease renders a page CropBox and charges the returned raster to
// the page's live-memory budget.
func RenderPageLease(ctx context.Context, page *pdf.Page, opts PageOptions) (*Raster, error) {
	if page == nil {
		return nil, errs.Wrap(errs.ErrClosed, "RND-PAGE", "render page", -1, 0, -1)
	}
	w, h := page.Size()
	return RenderRegionLease(ctx, page, displaylist.Rect{X1: w, Y1: h}, opts)
}

// RenderPage renders the page CropBox using RenderRegion.
func RenderPage(ctx context.Context, page *pdf.Page, opts PageOptions) (stdimage.Image, error) {
	if page == nil {
		return nil, errs.Wrap(errs.ErrClosed, "RND-PAGE", "render page", -1, 0, -1)
	}
	w, h := page.Size()
	return RenderRegion(ctx, page, displaylist.Rect{X1: w, Y1: h}, opts)
}

// EncodePNG writes img as a PNG stream.
func EncodePNG(w io.Writer, img stdimage.Image) error {
	if w == nil || img == nil {
		return errs.Wrap(errs.ErrCorrupt, "RND-PNG", "encode PNG", -1, 0, -1)
	}
	return png.Encode(w, img)
}

// EncodeJPEG writes img as a JPEG stream. Alpha is encoded as opaque; callers
// should render with a white or explicit background first.
func EncodeJPEG(w io.Writer, img stdimage.Image, quality int) error {
	if w == nil || img == nil {
		return errs.Wrap(errs.ErrCorrupt, "RND-JPEG", "encode JPEG", -1, 0, -1)
	}
	if quality <= 0 || quality > 100 {
		quality = 90
	}
	return jpeg.Encode(w, img, &jpeg.Options{Quality: quality})
}
