package pdf

import (
	"context"
	"fmt"

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
	DomainEdit    Domain = "edit"
	DomainExport  Domain = "export"
	DomainVerify  Domain = "verify"
)

// DomainReport is one preflight domain. Notes explain approximations and gaps.
type DomainReport struct {
	Status   CapabilityStatus
	Outcome  Outcome
	Notes    []string
	Features []Capability
}

// PreflightReport is a per-document scan of capability and execution status.
// It is not a proof of pixel or conformance correctness.
type PreflightReport struct {
	Pages       int
	Scanned     int
	Encrypted   bool
	Repaired    bool
	Domains     map[Domain]*DomainReport
	Diagnostics []Diagnostic
}

// Preflight scans every page and reports Parse/Extract/Render/Edit/Export/
// Verify status. It can be expensive for large documents.
func (d *Document) Preflight(ctx context.Context) (*PreflightReport, error) {
	if d == nil || d.inner == nil {
		return nil, errs.Wrap(errs.ErrClosed, "DOC-CLOSED", "preflight", -1, 0, -1)
	}
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap(errs.ErrCanceled, "CNT-CANCELED", "preflight", -1, 0, -1)
	}
	r := &PreflightReport{Pages: d.NumPages(), Encrypted: d.Encrypted(), Repaired: d.Repaired()}
	r.Diagnostics = d.Diagnostics()
	r.Domains = map[Domain]*DomainReport{
		DomainParse:   {Status: CapabilitySupported, Outcome: OutcomeSuccess},
		DomainExtract: {Status: CapabilitySupported, Outcome: OutcomeSuccess},
		DomainRender:  {Status: CapabilityApproximate, Outcome: OutcomeSuccess},
		DomainEdit:    {Status: CapabilityUnsupported, Outcome: OutcomeNotRun},
		DomainExport:  {Status: CapabilitySupported, Outcome: OutcomeNotRun},
		DomainVerify:  {Status: CapabilityUnsupported, Outcome: OutcomeNotRun},
	}
	var missingUnicode, failedPages, images, shadings int
	for i := 0; i < r.Pages; i++ {
		if err := ctx.Err(); err != nil {
			return nil, errs.Wrap(errs.ErrCanceled, "CNT-CANCELED", "preflight", -1, 0, i)
		}
		p, err := d.Page(i)
		if err != nil {
			failedPages++
			continue
		}
		dl, err := p.DisplayList(ctx)
		if err != nil {
			failedPages++
			continue
		}
		r.Scanned++
		var scan func([]displaylist.Item)
		scan = func(items []displaylist.Item) {
			for _, it := range items {
				switch v := it.(type) {
				case *displaylist.TextRun:
					for _, g := range v.Glyphs {
						if !g.UnicodeOK {
							missingUnicode++
						}
					}
				case *displaylist.Image:
					images++
				case *displaylist.Shading:
					shadings++
				case *displaylist.Group:
					scan(v.Items)
				}
			}
		}
		scan(dl.Items)
	}
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
	r.Domains[DomainRender].Notes = append(r.Domains[DomainRender].Notes, "sRGB compositing; no tile/executor or print color")
	if images > 0 {
		r.Domains[DomainRender].Notes = append(r.Domains[DomainRender].Notes, fmt.Sprintf("%d images", images))
	}
	if shadings > 0 {
		r.Domains[DomainRender].Notes = append(r.Domains[DomainRender].Notes, fmt.Sprintf("%d shadings", shadings))
	}
	r.Domains[DomainExport].Notes = append(r.Domains[DomainExport].Notes, "PNG and JPEG encoders available; no SVG/HTML/PS/EPS")
	r.Domains[DomainEdit].Notes = append(r.Domains[DomainEdit].Notes, "write and form editing are not implemented")
	r.Domains[DomainVerify].Notes = append(r.Domains[DomainVerify].Notes, "signature verification is not implemented")
	return r, nil
}

func featuresFor(domain Domain) []Capability {
	areas := map[Domain]map[string]bool{
		DomainParse:   {"parse": true, "security": true},
		DomainExtract: {"text": true, "document": true, "font": true},
		DomainRender:  {"render": true, "color": true},
		DomainEdit:    {"edit": true},
	}
	out := make([]Capability, 0)
	for _, c := range Capabilities() {
		if areas[domain][c.Area] {
			out = append(out, c)
		}
	}
	return out
}
