package render

import (
	"context"
	"runtime"
	"sync"

	"github.com/F31/go-pdf"
	"github.com/F31/go-pdf/internal/errs"
)

// PageResult is one page task result. Raster is non-nil only on success and
// must be released by the caller.
type PageResult struct {
	Index  int
	Raster *Raster
	Err    error
}

// RenderPages renders pages with a bounded worker pool. Empty pages means all
// pages. Results preserve the requested order; individual page failures are
// reported in PageResult.Err. The caller owns and must Release every Raster.
func RenderPages(ctx context.Context, doc *pdf.Document, pages []int, opts PageOptions, workers int) ([]PageResult, error) {
	if doc == nil {
		return nil, errs.Wrap(errs.ErrClosed, "DOC-CLOSED", "render pages", -1, 0, -1)
	}
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap(errs.ErrCanceled, "RND-CANCELED", "render pages", -1, 0, -1)
	}
	if len(pages) == 0 {
		pages = make([]int, doc.NumPages())
		for i := range pages {
			pages[i] = i
		}
	}
	for _, idx := range pages {
		if idx < 0 || idx >= doc.NumPages() {
			return nil, errs.Wrap(errs.ErrCorrupt, "RND-PAGE-RANGE", "render pages", -1, 0, idx)
		}
	}
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	if workers > len(pages) {
		workers = len(pages)
	}
	if workers < 1 {
		workers = 1
	}
	results := make([]PageResult, len(pages))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				results[i].Index = pages[i]
				p, err := doc.Page(pages[i])
				if err != nil {
					results[i].Err = err
					continue
				}
				r, err := RenderPageLease(ctx, p, opts)
				results[i].Raster = r
				results[i].Err = err
			}
		}()
	}
	for i := range pages {
		select {
		case jobs <- i:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			releaseResults(results)
			return results, errs.Wrap(errs.ErrCanceled, "RND-CANCELED", "render pages", -1, 0, -1)
		}
	}
	close(jobs)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		releaseResults(results)
		return results, errs.Wrap(errs.ErrCanceled, "RND-CANCELED", "render pages", -1, 0, -1)
	}
	return results, nil
}

func releaseResults(results []PageResult) {
	for i := range results {
		if results[i].Raster != nil {
			results[i].Raster.Release()
			results[i].Raster = nil
		}
	}
}
