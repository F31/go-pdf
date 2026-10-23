// Package doc implements the document layer: catalog, page tree, inherited
// attributes, page geometry, and metadata.
package doc

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"

	"github.com/F31/go-pdf/internal/budget"
	pdfcolor "github.com/F31/go-pdf/internal/color"
	"github.com/F31/go-pdf/internal/diag"
	"github.com/F31/go-pdf/internal/errs"
	"github.com/F31/go-pdf/internal/font"
	pdfimage "github.com/F31/go-pdf/internal/image"
	"github.com/F31/go-pdf/internal/syntax"
)

type Box struct {
	X0, Y0, X1, Y1 float64
}

func (b Box) Width() float64  { return b.X1 - b.X0 }
func (b Box) Height() float64 { return b.Y1 - b.Y0 }

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

type Options struct {
	Repair        bool
	IncludeHidden bool
	PreserveForms bool
	Decoders      pdfimage.DecoderLookup
	ColorManager  pdfcolor.Manager
	Tracker       *budget.Tracker
	Diag          *diag.Collector
}

type Document struct {
	file    *syntax.File
	catalog syntax.Dict
	info    Info
	pages   []*Page
	version string
	opts    Options
	fonts   *font.Cache
	oc      map[syntax.Ref]bool
	ocNames map[syntax.Ref]string

	mu      sync.RWMutex
	leaseMu sync.Mutex
	leases  []func()
	closed  bool
}

type Page struct {
	doc       *Document
	ref       syntax.Ref
	index     int
	dict      syntax.Dict
	resources syntax.Dict
	media     Box
	crop      Box
	rotate    int
	userUnit  float64
	width     float64
	height    float64
	matrix    [6]float64
}

func Open(ctx context.Context, file *syntax.File, opts Options) (*Document, error) {
	d := &Document{file: file, opts: opts, fonts: font.NewCache(opts.Tracker)}
	catalog, err := file.ResolveRoot(ctx)
	if err != nil {
		return nil, err
	}
	d.catalog = catalog
	if err := d.initialize(ctx); err != nil {
		if opts.Repair && !file.Repaired() && errors.Is(err, errs.ErrCorrupt) {
			if rerr := file.Rebuild(ctx); rerr == nil {
				if c2, e2 := file.ResolveRoot(ctx); e2 == nil {
					d.catalog = c2
					d.info = Info{}
					d.pages = nil
					d.fonts.Close()
					d.releaseLeases()
					d.fonts = font.NewCache(opts.Tracker)
					if e3 := d.initialize(ctx); e3 == nil {
						return d, nil
					} else {
						err = e3
					}
				}
			}
		}
		d.fonts.Close()
		d.releaseLeases()
		return nil, err
	}
	return d, nil
}

func (d *Document) initialize(ctx context.Context) error {
	file := d.file
	d.loadOC(ctx)
	if t, _ := d.catalog.NameValue("Type"); t != "" && t != "Catalog" {
		file.Diag().Add(diag.Diagnostic{Level: diag.Warn, Code: "DOC-CATALOG-TYPE", Msg: "catalog Type is not Catalog", Page: -1, Offset: -1})
	}
	d.version = file.HeaderVersion()
	if v, ok := d.catalog.NameValue("Version"); ok {
		if compareVersion(v, d.version) > 0 {
			file.Diag().Add(diag.Diagnostic{Level: diag.Info, Code: "DOC-VERSION-MISMATCH", Msg: "catalog version is newer than header", Page: -1, Offset: -1})
			d.version = v
		}
	}
	if err := d.loadInfo(ctx); err != nil {
		return err
	}
	return d.loadPages(ctx)
}

func compareVersion(a, b string) int {
	pa := strings.SplitN(a, ".", 2)
	pb := strings.SplitN(b, ".", 2)
	ai, _ := strconv.Atoi(pa[0])
	bi, _ := strconv.Atoi(pb[0])
	if ai != bi {
		if ai < bi {
			return -1
		}
		return 1
	}
	am, bm := 0, 0
	if len(pa) > 1 {
		am, _ = strconv.Atoi(pa[1])
	}
	if len(pb) > 1 {
		bm, _ = strconv.Atoi(pb[1])
	}
	switch {
	case am < bm:
		return -1
	case am > bm:
		return 1
	}
	return 0
}

func (d *Document) NumPages() int { return len(d.pages) }

func (d *Document) Page(i int) (*Page, error) {
	d.mu.RLock()
	closed := d.closed
	d.mu.RUnlock()
	if closed {
		return nil, errs.Wrap(errs.ErrClosed, "DOC-CLOSED", "get page", -1, 0, i)
	}
	if i < 0 || i >= len(d.pages) {
		return nil, errs.Wrap(errs.ErrCorrupt, "DOC-PAGE-RANGE", "get page", -1, 0, i)
	}
	return d.pages[i], nil
}

func (d *Document) Info() Info       { return d.info }
func (d *Document) Producer() string { return d.info.Producer }
func (d *Document) Version() string  { return d.version }
func (d *Document) Repaired() bool   { return d.file.Repaired() }

func (d *Document) Diagnostics() []diag.Diagnostic {
	return d.file.Diag().All()
}

func (d *Document) Encrypted() bool {
	_, ok := d.file.Trailer()["Encrypt"]
	return ok
}

func (d *Document) Close() error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil
	}
	d.closed = true
	d.mu.Unlock()
	d.fonts.Close()
	d.releaseLeases()
	return d.file.Close()
}

func (d *Document) acquireLease(n int64) (func(), error) {
	if d == nil || d.opts.Tracker == nil || n <= 0 {
		return func() {}, nil
	}
	release, err := d.opts.Tracker.Acquire(n)
	if err != nil {
		return nil, err
	}
	d.leaseMu.Lock()
	d.leases = append(d.leases, release)
	d.leaseMu.Unlock()
	return release, nil
}

func (d *Document) releaseLeases() {
	d.leaseMu.Lock()
	leases := d.leases
	d.leases = nil
	d.leaseMu.Unlock()
	for i := len(leases) - 1; i >= 0; i-- {
		leases[i]()
	}
}

func (d *Document) loadOC(ctx context.Context) {
	d.oc = nil
	d.ocNames = nil
	obj, err := d.file.Resolve(ctx, d.catalog.Get("OCProperties"))
	if err != nil {
		return
	}
	ocp, ok := obj.(syntax.Dict)
	if !ok {
		return
	}
	d.ocNames = map[syntax.Ref]string{}
	visible := map[syntax.Ref]bool{}
	if arr, ok := ocp.Get("OCGs").(syntax.Array); ok {
		for _, item := range arr {
			ref, ok := item.(syntax.Ref)
			if !ok {
				continue
			}
			visible[ref] = true
			o, err := d.file.Resolve(ctx, ref)
			if err != nil {
				continue
			}
			if dict, ok := o.(syntax.Dict); ok {
				if n, ok := dict.Get("Name").(syntax.String); ok {
					d.ocNames[ref] = decodeTextString(n)
				} else if n, ok := dict.Get("Name").(syntax.Name); ok {
					d.ocNames[ref] = string(n)
				}
			}
		}
	}
	if d.opts.IncludeHidden {
		return
	}
	dconf, _ := d.file.Resolve(ctx, ocp.Get("D"))
	conf, _ := dconf.(syntax.Dict)
	base := "ON"
	if n, ok := conf.NameValue("BaseState"); ok {
		base = n
	}
	if base == "OFF" {
		for ref := range visible {
			visible[ref] = false
		}
	}
	if arr, ok := conf.Get("ON").(syntax.Array); ok {
		for _, item := range arr {
			if ref, ok := item.(syntax.Ref); ok {
				visible[ref] = true
			}
		}
	}
	if arr, ok := conf.Get("OFF").(syntax.Array); ok {
		for _, item := range arr {
			if ref, ok := item.(syntax.Ref); ok {
				visible[ref] = false
			}
		}
	}
	d.oc = visible
}

func (d *Document) loadInfo(ctx context.Context) error {
	infoObj, ok := d.file.Trailer()["Info"]
	if !ok {
		d.info.XMP = d.loadXMP(ctx)
		applyXMP(&d.info, d.info.XMP)
		return nil
	}
	o, err := d.file.Resolve(ctx, infoObj)
	if err != nil {
		return err
	}
	dict, ok := o.(syntax.Dict)
	if !ok {
		d.file.Diag().Add(diag.Diagnostic{Level: diag.Warn, Code: "DOC-INFO-TYPE", Msg: "Info is not a dictionary", Page: -1, Offset: -1})
		return nil
	}
	d.info = Info{
		Title:        decodeText(dict.Get("Title")),
		Author:       decodeText(dict.Get("Author")),
		Subject:      decodeText(dict.Get("Subject")),
		Keywords:     decodeText(dict.Get("Keywords")),
		Creator:      decodeText(dict.Get("Creator")),
		Producer:     decodeText(dict.Get("Producer")),
		CreationDate: decodeText(dict.Get("CreationDate")),
		ModDate:      decodeText(dict.Get("ModDate")),
	}
	d.info.XMP = d.loadXMP(ctx)
	applyXMP(&d.info, d.info.XMP)
	return nil
}

func (d *Document) loadXMP(ctx context.Context) []byte {
	v, ok := d.catalog["Metadata"]
	if !ok {
		return nil
	}
	o, err := d.file.Resolve(ctx, v)
	if err != nil {
		return nil
	}
	meta, ok := o.(*syntax.Stream)
	if !ok {
		return nil
	}
	xmp, err := d.file.DecodeStream(ctx, meta)
	if err != nil {
		d.file.Diag().Add(diag.Diagnostic{Level: diag.Warn, Code: "DOC-XMP-DECODE", Msg: "failed to decode XMP", Page: -1, Offset: -1})
		return nil
	}
	return xmp
}

func decodeText(o syntax.Object) string {
	var b []byte
	switch v := o.(type) {
	case syntax.String:
		b = []byte(v)
	case syntax.Name:
		return string(v)
	default:
		return ""
	}
	if len(b) >= 2 {
		if b[0] == 0xFE && b[1] == 0xFF {
			return decodeUTF16BE(b[2:])
		}
		if b[0] == 0xFF && b[1] == 0xFE {
			return decodeUTF16LE(b[2:])
		}
	}
	var sb strings.Builder
	for _, c := range b {
		sb.WriteRune(pdfDocRune(c))
	}
	return sb.String()
}

func decodeUTF16BE(b []byte) string {
	var sb strings.Builder
	for i := 0; i+1 < len(b); i += 2 {
		sb.WriteRune(rune(b[i])<<8 | rune(b[i+1]))
	}
	return sb.String()
}

func decodeUTF16LE(b []byte) string {
	var sb strings.Builder
	for i := 0; i+1 < len(b); i += 2 {
		sb.WriteRune(rune(b[i+1])<<8 | rune(b[i]))
	}
	return sb.String()
}

// pdfDocRune implements the ASCII/Latin-1 subset of PDFDocEncoding. The
// remaining control-range mappings are rare in metadata and are treated as
// Latin-1 until corpus evidence justifies a full table.
func pdfDocRune(c byte) rune {
	switch c {
	case 0x18:
		return '\u02d8'
	case 0x19:
		return '\u02c7'
	case 0x1a:
		return '\u02c6'
	case 0x1b:
		return '\u02d9'
	case 0x1c:
		return '\u02dd'
	case 0x1d:
		return '\u02db'
	case 0x1e:
		return '\u02da'
	case 0x1f:
		return '\u02dc'
	case 0x80:
		return '\u2022'
	case 0x81:
		return '\u2020'
	case 0x82:
		return '\u2021'
	case 0x83:
		return '\u2026'
	case 0x84:
		return '\u2014'
	case 0x85:
		return '\u2013'
	case 0x86:
		return '\u0192'
	case 0x87:
		return '\u2044'
	case 0x88:
		return '\u2039'
	case 0x89:
		return '\u203a'
	case 0x8a:
		return '\u2212'
	case 0x8b:
		return '\u2030'
	case 0x8c:
		return '\u201e'
	case 0x8d:
		return '\u201c'
	case 0x8e:
		return '\u201d'
	case 0x8f:
		return '\u2018'
	case 0x90:
		return '\u2019'
	case 0x91:
		return '\u201a'
	case 0x92:
		return '\u2122'
	case 0x93:
		return '\ufb01'
	case 0x94:
		return '\ufb02'
	case 0x95:
		return '\u0141'
	case 0x96:
		return '\u0152'
	case 0x97:
		return '\u0160'
	case 0x98:
		return '\u0178'
	case 0x99:
		return '\u017d'
	case 0x9a:
		return '\u0131'
	case 0x9b:
		return '\u0142'
	case 0x9c:
		return '\u0153'
	case 0x9d:
		return '\u0161'
	case 0x9e:
		return '\u017e'
	case 0xa0:
		return '\u20ac'
	}
	return rune(c)
}

// Unsupported records a diagnostic and returns an ErrUnsupported error.
func (d *Document) Unsupported(code, op string, page int) error {
	d.file.Diag().Add(diag.Diagnostic{Level: diag.Degraded, Code: code, Msg: op + " is not implemented in G0", Page: page, Offset: -1})
	return errs.Wrap(errs.ErrUnsupported, code, op, -1, 0, page)
}

func (d *Document) DiagnosticsForPage(page int) []diag.Diagnostic {
	if d == nil || d.file == nil {
		return nil
	}
	return d.file.Diag().ForPage(page)
}
