// Package pdf is the public entry point for go-pdf. G0 implements document
// opening, page traversal, page geometry, metadata, budgets and diagnostics.
package pdf

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"sync/atomic"

	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/internal/budget"
	"github.com/F31/go-pdf/internal/diag"
	"github.com/F31/go-pdf/internal/doc"
	"github.com/F31/go-pdf/internal/errs"
	pdfimage "github.com/F31/go-pdf/internal/image"
	"github.com/F31/go-pdf/internal/syntax"
)

// Document is a parsed PDF document. It is safe for concurrent use. Page
// handles returned by Page are intended for single-goroutine use.
type Document struct {
	inner   *doc.Document
	tracker *budget.Tracker
	diag    *diag.Collector
	budget  *budget.Budget

	structureMu   sync.Mutex
	structure     []StructElement
	structureDone bool
	structureErr  error

	release     func()
	releaseOnce sync.Once
	dataRelease func()
	dataOnce    sync.Once
	closed      atomic.Bool
}

// ResourceUsage is a point-in-time view of document resource accounting.
type ResourceUsage struct {
	LiveBytes    int64
	PeakBytes    int64
	DecodedBytes int64
	Objects      int64
	MaxLiveBytes int64
}

func OpenFile(ctx context.Context, path string, opts ...Option) (*Document, error) {
	o, err := parseOptions(opts)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errs.Wrap(err, "SYN-OPEN", "open file", -1, 0, -1)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, errs.Wrap(err, "SYN-STAT", "stat file", -1, 0, -1)
	}
	if st.Size() > o.budget.MaxFileSize {
		return nil, errs.Wrap(errs.ErrBudget, "BUD-FILE-SIZE", "open file", -1, 0, -1)
	}
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap(errs.ErrCanceled, "SYN-CANCELED", "open file", -1, 0, -1)
	}
	data, dataRelease, err := mapReadOnly(f, st.Size())
	if err != nil {
		return nil, errs.Wrap(err, "SYN-READ", "read file", -1, 0, -1)
	}
	doc, err := openData(ctx, data, o)
	if err != nil {
		dataRelease()
		return nil, err
	}
	doc.dataRelease = dataRelease
	return doc, nil
}

func Open(ctx context.Context, r io.ReaderAt, size int64, opts ...Option) (*Document, error) {
	o, err := parseOptions(opts)
	if err != nil {
		return nil, err
	}
	if r == nil || size < 0 {
		return nil, errs.Wrap(errs.ErrCorrupt, "SYN-SIZE", "open reader", -1, 0, -1)
	}
	if size > o.budget.MaxFileSize {
		return nil, errs.Wrap(errs.ErrBudget, "BUD-FILE-SIZE", "open reader", -1, 0, -1)
	}
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap(errs.ErrCanceled, "SYN-CANCELED", "open reader", -1, 0, -1)
	}
	if f, ok := r.(*os.File); ok {
		data, release, err := mapReadOnly(f, size)
		if err != nil {
			return nil, errs.Wrap(err, "SYN-READ", "read reader", -1, 0, -1)
		}
		doc, err := openData(ctx, data, o)
		if err != nil {
			release()
			return nil, err
		}
		doc.dataRelease = release
		return doc, nil
	}
	data, release, err := mapReaderAt(ctx, r, size)
	if err != nil {
		if ctx.Err() != nil {
			return nil, errs.Wrap(errs.ErrCanceled, "SYN-CANCELED", "read reader", -1, 0, -1)
		}
		return nil, errs.Wrap(err, "SYN-READ", "read reader", -1, 0, -1)
	}
	doc, err := openData(ctx, data, o)
	if err != nil {
		release()
		return nil, err
	}
	doc.dataRelease = release
	return doc, nil
}

func readAll(r io.ReaderAt, size int64) ([]byte, error) {
	if size == 0 {
		return []byte{}, nil
	}
	if size > int64(int(^uint(0)>>1)) {
		return nil, errs.Wrap(errs.ErrBudget, "BUD-FILE-SIZE", "read input", -1, 0, -1)
	}
	buf := make([]byte, int(size))
	if _, err := io.ReadFull(io.NewSectionReader(r, 0, size), buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func readAllContext(ctx context.Context, r io.ReaderAt, size int64) ([]byte, error) {
	if size == 0 {
		return []byte{}, nil
	}
	if size > int64(int(^uint(0)>>1)) {
		return nil, errs.Wrap(errs.ErrBudget, "BUD-FILE-SIZE", "read input", -1, 0, -1)
	}
	buf := make([]byte, int(size))
	for off := 0; off < len(buf); {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n := len(buf) - off
		if n > 1<<20 {
			n = 1 << 20
		}
		read, err := r.ReadAt(buf[off:off+n], int64(off))
		off += read
		if err != nil {
			if errors.Is(err, io.EOF) && off == len(buf) {
				break
			}
			return nil, err
		}
		if read == 0 {
			return nil, io.ErrNoProgress
		}
	}
	return buf, nil
}

func openData(ctx context.Context, data []byte, o options) (*Document, error) {
	tracker := budget.NewTracker(o.budget)
	collector := diag.New(diag.DefaultCapacity)
	ctx = budget.WithBudget(ctx, o.budget)
	ctx = budget.WithTracker(ctx, tracker)
	ctx = diag.WithCollector(ctx, collector)

	release, err := tracker.Acquire(int64(len(data)))
	if err != nil {
		return nil, err
	}
	file, err := syntax.Open(ctx, data, syntax.Options{
		Repair: o.repair, Password: o.password, Tracker: tracker, Diag: collector,
	})
	if err != nil {
		release()
		return nil, err
	}
	var decoders pdfimage.DecoderLookup
	if o.decoders != nil {
		reg := o.decoders
		decoders = func(ctx context.Context, enc displaylist.ImageEncoding, info displaylist.ImageInfo) (pdfimage.ExternalDecoder, error) {
			return reg.decoder(ctx, enc, info)
		}
	}
	decoders = chainDecoderLookups(decoders, extensionDecoderLookup(o.extensions))
	inner, err := doc.Open(ctx, file, doc.Options{
		Repair: o.repair, IncludeHidden: o.hidden, PreserveForms: o.forms,
		Decoders: decoders, ColorManager: o.colorManager, Tracker: tracker, Diag: collector,
	})
	if err != nil {
		release()
		_ = file.Close()
		return nil, err
	}
	return &Document{inner: inner, tracker: tracker, diag: collector, budget: o.budget, release: release}, nil
}

func (d *Document) NumPages() int {
	if d == nil || d.inner == nil {
		return 0
	}
	return d.inner.NumPages()
}

func (d *Document) Page(i int) (*Page, error) {
	if d == nil || d.inner == nil {
		return nil, errs.Wrap(errs.ErrClosed, "DOC-CLOSED", "get page", -1, 0, i)
	}
	p, err := d.inner.Page(i)
	if err != nil {
		return nil, err
	}
	return &Page{inner: p, doc: d}, nil
}

func (d *Document) Info() Info {
	if d == nil || d.inner == nil {
		return Info{}
	}
	i := d.inner.Info()
	return Info{
		Title: i.Title, Author: i.Author, Subject: i.Subject,
		Keywords: i.Keywords, Creator: i.Creator, Producer: i.Producer,
		CreationDate: i.CreationDate, ModDate: i.ModDate, XMP: i.XMP,
	}
}

func (d *Document) Producer() string {
	if d == nil || d.inner == nil {
		return ""
	}
	return d.inner.Producer()
}

func (d *Document) Version() string {
	if d == nil || d.inner == nil {
		return ""
	}
	return d.inner.Version()
}

func (d *Document) Repaired() bool {
	if d == nil || d.inner == nil {
		return false
	}
	return d.inner.Repaired()
}

// PageLabelRange is one decoded page label range. Start is zero-based.
type PageLabelRange struct {
	Start  int
	Style  string
	Prefix string
	First  int
}

// PageLabelRanges returns decoded /PageLabels ranges in start order. An empty
// result means the document uses the default decimal labels.
func (d *Document) PageLabelRanges(ctx context.Context) ([]PageLabelRange, error) {
	if d == nil || d.inner == nil {
		return nil, nil
	}
	src := d.inner.PageLabelRanges(ctx)
	out := make([]PageLabelRange, len(src))
	for i, r := range src {
		out[i] = PageLabelRange{Start: r.Start, Style: r.Style, Prefix: r.Prefix, First: r.First}
	}
	return out, nil
}

// PageLabel returns the label for a zero-based page index.
func (d *Document) PageLabel(ctx context.Context, index int) (string, error) {
	if d == nil || d.inner == nil {
		return "", nil
	}
	return d.inner.PageLabel(ctx, index), nil
}

func (d *Document) Encrypted() bool {
	if d == nil || d.inner == nil {
		return false
	}
	return d.inner.Encrypted()
}

func (d *Document) Diagnostics() []Diagnostic {
	if d == nil || d.inner == nil {
		return nil
	}
	src := d.inner.Diagnostics()
	out := make([]Diagnostic, len(src))
	for i, x := range src {
		out[i] = Diagnostic{Level: Level(x.Level), Code: x.Code, Msg: x.Msg, Page: x.Page, Obj: x.Obj, Offset: x.Offset}
	}
	return out
}

func (d *Document) Close() error {
	if d == nil || d.inner == nil {
		return nil
	}
	d.closed.Store(true)
	err := d.inner.Close()
	d.releaseOnce.Do(func() {
		if d.release != nil {
			d.release()
		}
	})
	d.dataOnce.Do(func() {
		if d.dataRelease != nil {
			d.dataRelease()
		}
	})
	return err
}

// ResourceUsage reports live, peak, decoded and object counts. LiveBytes drops
// after Close; PeakBytes remains as the high-water mark.
func (d *Document) ResourceUsage() ResourceUsage {
	if d == nil || d.tracker == nil {
		return ResourceUsage{}
	}
	return ResourceUsage{
		LiveBytes: d.tracker.Live(), PeakBytes: d.tracker.Peak(),
		DecodedBytes: d.tracker.Decoded(), Objects: d.tracker.Objects(),
		MaxLiveBytes: d.tracker.Budget().MaxLiveBytes,
	}
}

type Info struct {
	Title        string
	Author       string
	Subject      string
	Keywords     string
	Creator      string
	Producer     string
	CreationDate string
	ModDate      string
	XMP          []byte
}
