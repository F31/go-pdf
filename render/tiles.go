package render

import (
	"context"
	"image"
	"image/draw"
	"math"

	"github.com/F31/go-pdf"
	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/internal/errs"
)

// Tile is one rendered tile. X and Y are the tile origin in the full raster;
// Image may be smaller at right/bottom edges. Rect is the page-space box.
type Tile struct {
	Row, Col int
	X, Y     int
	Rect     displaylist.Rect
	Image    image.Image
}

// TilePlan describes a fixed-pixel tiling of a rendered region.
type TilePlan struct {
	Width, Height int
	TilePixels    int
	Columns, Rows int
	Scale         float64
}

// PlanTiles validates a region and computes its tile grid. tilePixels <= 0
// uses 1024 and is clamped to 16..8192.
func PlanTiles(box displaylist.Rect, opts PageOptions, tilePixels int) (TilePlan, error) {
	w, h, scale, err := regionSpec(box, opts)
	if err != nil {
		return TilePlan{}, err
	}
	if tilePixels <= 0 {
		tilePixels = 1024
	}
	if tilePixels < 16 {
		tilePixels = 16
	}
	if tilePixels > 8192 {
		tilePixels = 8192
	}
	return TilePlan{
		Width: w, Height: h, TilePixels: tilePixels, Scale: scale,
		Columns: (w + tilePixels - 1) / tilePixels,
		Rows:    (h + tilePixels - 1) / tilePixels,
	}, nil
}

// RenderTiles renders a region in bounded pixel tiles. The callback receives a
// tile whose output lease is released after it returns; callers that retain a
// tile must copy it.
func RenderTiles(ctx context.Context, page *pdf.Page, box displaylist.Rect, opts PageOptions, tilePixels int, fn func(Tile) error) error {
	if page == nil {
		return errs.Wrap(errs.ErrClosed, "RND-PAGE", "render tiles", -1, 0, -1)
	}
	if fn == nil {
		return errs.Wrap(errs.ErrCorrupt, "RND-TILE", "render tiles", -1, 0, -1)
	}
	if err := ctx.Err(); err != nil {
		return errs.Wrap(errs.ErrCanceled, "RND-CANCELED", "render tiles", -1, 0, -1)
	}
	plan, err := PlanTiles(box, opts, tilePixels)
	if err != nil {
		return err
	}
	dl, err := page.DisplayList(ctx)
	if err != nil {
		return err
	}
	scale := plan.Scale
	for y := 0; y < plan.Height; y += plan.TilePixels {
		y2 := y + plan.TilePixels
		if y2 > plan.Height {
			y2 = plan.Height
		}
		for x := 0; x < plan.Width; x += plan.TilePixels {
			x2 := x + plan.TilePixels
			if x2 > plan.Width {
				x2 = plan.Width
			}
			rect := displaylist.Rect{
				X0: box.X0 + float64(x)/scale, Y0: box.Y0 + float64(y)/scale,
				X1: box.X0 + float64(x2)/scale, Y1: box.Y0 + float64(y2)/scale,
			}
			r, err := renderRegionLeaseDL(ctx, page, rect, opts, dl)
			if err != nil {
				return err
			}
			tile := Tile{Row: y / plan.TilePixels, Col: x / plan.TilePixels, X: x, Y: y, Rect: rect, Image: r.Image}
			err = fn(tile)
			r.Release()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// RenderPageTiles is RenderTiles using the rotated page space.
func RenderPageTiles(ctx context.Context, page *pdf.Page, opts PageOptions, tilePixels int, fn func(Tile) error) error {
	if page == nil {
		return errs.Wrap(errs.ErrClosed, "RND-PAGE", "render page tiles", -1, 0, -1)
	}
	w, h := page.Size()
	return RenderTiles(ctx, page, displaylist.Rect{X1: w, Y1: h}, opts, tilePixels, fn)
}

// PlanTilesAdaptive chooses tilePixels from a target RGBA tile byte budget.
// maxTileBytes <= 0 uses 16 MiB. The result is clamped to 16..8192 pixels and
// does not include transient masks or decoded images.
func PlanTilesAdaptive(box displaylist.Rect, opts PageOptions, maxTileBytes int64) (TilePlan, error) {
	if maxTileBytes <= 0 {
		maxTileBytes = 16 << 20
	}
	pixels := int(math.Sqrt(float64(maxTileBytes) / 4))
	if pixels < 16 {
		pixels = 16
	}
	if pixels > 8192 {
		pixels = 8192
	}
	return PlanTiles(box, opts, pixels)
}

// RenderTilesTo renders into dst in bounded tiles. dst must be at least the
// planned raster size and is not charged by the page memory budget.
func RenderTilesTo(ctx context.Context, page *pdf.Page, box displaylist.Rect, opts PageOptions, tilePixels int, dst draw.Image) error {
	if dst == nil {
		return errs.Wrap(errs.ErrCorrupt, "RND-TILE", "render tiles to destination", -1, 0, -1)
	}
	plan, err := PlanTiles(box, opts, tilePixels)
	if err != nil {
		return err
	}
	b := dst.Bounds()
	if b.Dx() < plan.Width || b.Dy() < plan.Height {
		return errs.Wrap(errs.ErrCorrupt, "RND-TILE", "destination smaller than raster", -1, 0, -1)
	}
	return RenderTiles(ctx, page, box, opts, tilePixels, func(t Tile) error {
		sb := t.Image.Bounds()
		dr := image.Rect(b.Min.X+t.X, b.Min.Y+t.Y, b.Min.X+t.X+sb.Dx(), b.Min.Y+t.Y+sb.Dy())
		draw.Draw(dst, dr, t.Image, sb.Min, draw.Src)
		return nil
	})
}

// RenderPageTilesTo is RenderTilesTo using the rotated page space.
func RenderPageTilesTo(ctx context.Context, page *pdf.Page, opts PageOptions, tilePixels int, dst draw.Image) error {
	if page == nil {
		return errs.Wrap(errs.ErrClosed, "RND-PAGE", "render page tiles to destination", -1, 0, -1)
	}
	w, h := page.Size()
	return RenderTilesTo(ctx, page, displaylist.Rect{X1: w, Y1: h}, opts, tilePixels, dst)
}
