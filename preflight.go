package pdf

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/internal/errs"
)

// Outcome describes execution, separately from capability support.
type Outcome string

const (
	OutcomeSuccess Outcome = "success"
	OutcomePartial Outcome = "partial"
	OutcomeFailed  Outcome = "failed"
	OutcomeNotRun  Outcome = "not-run"
)

// Domain groups capability and outcome reporting.
type Domain string

const (
	DomainParse   Domain = "parse"
	DomainExtract Domain = "extract"
	DomainRender  Domain = "render"
)

// DomainReport is one preflight domain. Notes explain approximations and gaps.
type DomainReport struct {
	Status   CapabilityStatus `json:"status"`
	Outcome  Outcome          `json:"outcome"`
	Notes    []string         `json:"notes,omitempty"`
	Features []Capability     `json:"features,omitempty"`
}

// PreflightReport is a per-document scan of capability and execution status.
// It is not a proof of pixel or conformance correctness.
type PageFailure struct {
	Page int    `json:"page"`
	Code string `json:"code,omitempty"`
}

type PreflightReport struct {
	Pages       int                      `json:"pages"`
	Scanned     int                      `json:"scanned"`
	Failed      int                      `json:"failed"`
	FailedPages []int                    `json:"failedPages,omitempty"`
	Failures    []PageFailure            `json:"failures,omitempty"`
	Encrypted   bool                     `json:"encrypted"`
	Repaired    bool                     `json:"repaired"`
	Domains     map[Domain]*DomainReport `json:"domains"`
	Diagnostics []Diagnostic             `json:"diagnostics,omitempty"`
}

// PreflightOptions controls a preflight scan. Pages are 0-based; nil means all.
// Workers is capped by the document budget.
type PreflightOptions struct {
	Pages   []int
	Workers int
}

// Preflight scans every page and reports Parse/Extract/Render status.
func (d *Document) Preflight(ctx context.Context) (*PreflightReport, error) {
	return d.PreflightWithOptions(ctx, PreflightOptions{})
}

// PreflightWithOptions scans selected pages with bounded workers.
func (d *Document) PreflightWithOptions(ctx context.Context, opts PreflightOptions) (*PreflightReport, error) {
	if d == nil || d.inner == nil || d.closed.Load() {
		return nil, errs.Wrap(errs.ErrClosed, "DOC-CLOSED", "preflight", -1, 0, -1)
	}
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap(errs.ErrCanceled, "CNT-CANCELED", "preflight", -1, 0, -1)
	}
	pages, err := d.extractPages(opts.Pages)
	if err != nil {
		return nil, err
	}
	r := &PreflightReport{Pages: d.NumPages(), Encrypted: d.Encrypted(), Repaired: d.Repaired()}
	r.Domains = map[Domain]*DomainReport{
		DomainParse:   {Status: CapabilitySupported, Outcome: OutcomeSuccess},
		DomainExtract: {Status: CapabilitySupported, Outcome: OutcomeSuccess},
		DomainRender:  {Status: CapabilityApproximate, Outcome: OutcomeSuccess},
	}
	var missingUnicode, failedPages, images, shadings int
	workers := d.extractWorkers(opts.Workers, len(pages))
	if workers < 1 {
		workers = 1
	}
	type pageResult struct {
		page     int
		ok       bool
		code     string
		missing  int
		images   int
		shadings int
	}
	jobs := make(chan int)
	results := make(chan pageResult, workers)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				out := pageResult{page: i}
				p, err := d.Page(i)
				if err != nil {
					out.code = diagnosticCode(err)
					results <- out
					continue
				}
				dl, err := p.DisplayList(ctx)
				if err != nil {
					out.code = diagnosticCode(err)
					results <- out
					continue
				}
				out.ok = true
				var scan func([]displaylist.Item)
				scan = func(items []displaylist.Item) {
					for _, it := range items {
						switch v := it.(type) {
						case *displaylist.TextRun:
							for _, g := range v.Glyphs {
								if !g.UnicodeOK {
									out.missing++
								}
							}
						case *displaylist.Image:
							out.images++
						case *displaylist.Shading:
							out.shadings++
						case *displaylist.Group:
							scan(v.Items)
						}
					}
				}
				scan(dl.Items)
				results <- out
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, page := range pages {
			select {
			case jobs <- page:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()
	for out := range results {
		if !out.ok {
			failedPages++
			r.FailedPages = append(r.FailedPages, out.page)
			r.Failures = append(r.Failures, PageFailure{Page: out.page, Code: out.code})
			continue
		}
		r.Scanned++
		missingUnicode += out.missing
		images += out.images
		shadings += out.shadings
	}
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap(errs.ErrCanceled, "CNT-CANCELED", "preflight", -1, 0, -1)
	}
	sort.Ints(r.FailedPages)
	sort.Slice(r.Failures, func(i, j int) bool { return r.Failures[i].Page < r.Failures[j].Page })
	r.Failed = len(r.FailedPages)
	r.Diagnostics = d.Diagnostics()
	r.Domains[DomainExtract].Features = featuresFor(DomainExtract)
	r.Domains[DomainRender].Features = featuresFor(DomainRender)
	r.Domains[DomainParse].Features = featuresFor(DomainParse)
	if r.Repaired {
		r.Domains[DomainParse].Notes = append(r.Domains[DomainParse].Notes, "document was repaired")
	}
	if failedPages > 0 {
		r.Domains[DomainParse].Outcome = OutcomePartial
		r.Domains[DomainParse].Notes = append(r.Domains[DomainParse].Notes, fmt.Sprintf("%d pages failed to scan", failedPages))
	}
	if r.Scanned == 0 {
		r.Domains[DomainParse].Status = CapabilityUnknown
		r.Domains[DomainParse].Outcome = OutcomeFailed
	}
	if missingUnicode > 0 {
		r.Domains[DomainExtract].Status = CapabilityApproximate
		r.Domains[DomainExtract].Outcome = OutcomePartial
		r.Domains[DomainExtract].Notes = append(r.Domains[DomainExtract].Notes, fmt.Sprintf("%d glyphs lack trusted Unicode", missingUnicode))
	}
	r.Domains[DomainRender].Notes = append(r.Domains[DomainRender].Notes, "Preview screen sRGB; tiled executor available; print colour and full codecs require Extended")
	if images > 0 {
		r.Domains[DomainRender].Notes = append(r.Domains[DomainRender].Notes, fmt.Sprintf("%d images", images))
	}
	if shadings > 0 {
		r.Domains[DomainRender].Notes = append(r.Domains[DomainRender].Notes, fmt.Sprintf("%d shadings", shadings))
	}
	return r, nil
}

func featuresFor(domain Domain) []Capability {
	areas := map[Domain]map[string]bool{
		DomainParse:   {"parse": true, "security": true},
		DomainExtract: {"text": true, "document": true, "font": true},
		DomainRender:  {"render": true, "color": true},
	}
	out := make([]Capability, 0)
	for _, c := range Capabilities() {
		if areas[domain][c.Area] {
			out = append(out, c)
		}
	}
	return out
}

func diagnosticCode(err error) string {
	var pe *errs.Error
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}
