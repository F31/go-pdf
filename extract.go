package pdf

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/F31/go-pdf/internal/errs"
	pdftext "github.com/F31/go-pdf/text"
)

// ExtractOptions controls batch text extraction. Pages are 0-based; nil or
// empty means every page. Workers is capped by the document budget.
type ExtractOptions struct {
	Pages   []int
	Workers int
	Text    pdftext.Options
}

// PageText is one page result. Err is non-nil only for that page; other pages
// continue to run.
type PageText struct {
	Index int
	Text  *pdftext.Page
	Err   error
}

// ExtractResult preserves request order and reports per-page failures.
// FailedPages is sorted ascending and de-duplicated.
type ExtractResult struct {
	Pages       []PageText
	Failed      int
	FailedPages []int
	Failures    []PageFailure
}

// ExtractSummary is the final aggregate for streaming extraction. FailedPages
// is sorted ascending and de-duplicated. Processed counts pages passed to the
// callback; pages produced after a callback error are not included.
type ExtractSummary struct {
	Processed   int
	Succeeded   int
	Failed      int
	FailedPages []int
	Failures    []PageFailure
}

// PageTextFunc receives one page result. Calls are serialized; the page order
// follows worker completion, not request order. Use PageText.Index for mapping.
type PageTextFunc func(PageText) error

// ExtractText extracts text from multiple pages with bounded concurrency. It
// returns a context error only after workers have stopped; page failures are
// reported in ExtractResult.Pages.
func (d *Document) ExtractText(ctx context.Context, opts ExtractOptions) (*ExtractResult, error) {
	if d == nil || d.inner == nil || d.closed.Load() {
		return nil, errs.Wrap(errs.ErrClosed, "DOC-CLOSED", "extract text", -1, 0, -1)
	}
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap(errs.ErrCanceled, "DOC-CANCELED", "extract text", -1, 0, -1)
	}
	pages, err := d.extractPages(opts.Pages)
	if err != nil {
		return nil, err
	}
	result := &ExtractResult{Pages: make([]PageText, len(pages))}
	if len(pages) == 0 {
		return result, nil
	}
	workers := d.extractWorkers(opts.Workers, len(pages))
	type job struct{ pos, page int }
	jobs := make(chan job)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				out := &result.Pages[j.pos]
				out.Index = j.page
				if err := ctx.Err(); err != nil {
					out.Err = err
					continue
				}
				p, err := d.Page(j.page)
				if err != nil {
					out.Err = err
					continue
				}
				text, err := p.TextWithOptions(ctx, opts.Text)
				if err != nil {
					out.Err = err
					continue
				}
				out.Text = text
			}
		}()
	}
	for pos, page := range pages {
		jobs <- job{pos: pos, page: page}
	}
	close(jobs)
	wg.Wait()
	seen := make(map[int]bool)
	for i := range result.Pages {
		if result.Pages[i].Err == nil {
			continue
		}
		result.Failed++
		if !seen[result.Pages[i].Index] {
			seen[result.Pages[i].Index] = true
			result.FailedPages = append(result.FailedPages, result.Pages[i].Index)
			result.Failures = append(result.Failures, PageFailure{Page: result.Pages[i].Index, Code: diagnosticCode(result.Pages[i].Err)})
		}
	}
	sort.Ints(result.FailedPages)
	sort.Slice(result.Failures, func(i, j int) bool { return result.Failures[i].Page < result.Failures[j].Page })
	if err := ctx.Err(); err != nil {
		return result, errs.Wrap(errs.ErrCanceled, "DOC-CANCELED", "extract text", -1, 0, -1)
	}
	return result, nil
}

// ExtractTextEach streams page results to fn with bounded workers. It avoids
// retaining every page's text, which is important for large server-side jobs.
// Page results arrive in completion order; fn is called by one goroutine only.
// The returned summary is available even when ctx or the callback stops early.
func (d *Document) ExtractTextEach(ctx context.Context, opts ExtractOptions, fn PageTextFunc) (*ExtractSummary, error) {
	if d == nil || d.inner == nil || d.closed.Load() {
		return nil, errs.Wrap(errs.ErrClosed, "DOC-CLOSED", "extract text", -1, 0, -1)
	}
	if fn == nil {
		return nil, errors.New("pdf: nil page callback")
	}
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap(errs.ErrCanceled, "DOC-CANCELED", "extract text", -1, 0, -1)
	}
	pages, err := d.extractPages(opts.Pages)
	if err != nil {
		return nil, err
	}
	summary := &ExtractSummary{}
	if len(pages) == 0 {
		return summary, nil
	}
	workers := d.extractWorkers(opts.Workers, len(pages))
	type job struct{ page int }
	type result struct{ row PageText }
	jobs := make(chan job)
	results := make(chan result, workers)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				row := PageText{Index: j.page}
				if err := runCtx.Err(); err != nil {
					row.Err = err
				} else if p, err := d.Page(j.page); err != nil {
					row.Err = err
				} else if text, err := p.TextWithOptions(runCtx, opts.Text); err != nil {
					row.Err = err
				} else {
					row.Text = text
				}
				select {
				case results <- result{row: row}:
				case <-runCtx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, page := range pages {
			select {
			case jobs <- job{page: page}:
			case <-runCtx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()
	var callbackErr error
	seenFailed := make(map[int]bool)
	for item := range results {
		if callbackErr != nil {
			continue
		}
		summary.Processed++
		if item.row.Err != nil {
			summary.Failed++
			if !seenFailed[item.row.Index] {
				seenFailed[item.row.Index] = true
				summary.FailedPages = append(summary.FailedPages, item.row.Index)
				summary.Failures = append(summary.Failures, PageFailure{Page: item.row.Index, Code: diagnosticCode(item.row.Err)})
			}
		} else {
			summary.Succeeded++
		}
		if err := fn(item.row); err != nil {
			callbackErr = err
			cancel()
		}
	}
	sort.Ints(summary.FailedPages)
	sort.Slice(summary.Failures, func(i, j int) bool { return summary.Failures[i].Page < summary.Failures[j].Page })
	if callbackErr != nil {
		return summary, callbackErr
	}
	if err := ctx.Err(); err != nil {
		return summary, errs.Wrap(errs.ErrCanceled, "DOC-CANCELED", "extract text", -1, 0, -1)
	}
	return summary, nil
}

func (d *Document) extractWorkers(requested, pages int) int {
	workers := requested
	if workers <= 0 {
		workers = 4
	}
	if d.budget != nil && d.budget.MaxConcurrency > 0 && workers > d.budget.MaxConcurrency {
		workers = d.budget.MaxConcurrency
	}
	if workers > pages {
		workers = pages
	}
	if workers < 1 {
		workers = 1
	}
	return workers
}

func (d *Document) extractPages(pages []int) ([]int, error) {
	if len(pages) == 0 {
		out := make([]int, d.NumPages())
		for i := range out {
			out[i] = i
		}
		return out, nil
	}
	out := make([]int, len(pages))
	for i, page := range pages {
		if page < 0 || page >= d.NumPages() {
			return nil, fmt.Errorf("pdf: page %d out of range 0..%d", page, d.NumPages()-1)
		}
		out[i] = page
	}
	return out, nil
}
