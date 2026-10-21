package doc

import (
	"context"

	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/internal/budget"
	"github.com/F31/go-pdf/internal/content"
	"github.com/F31/go-pdf/internal/diag"
	"github.com/F31/go-pdf/internal/errs"
	"github.com/F31/go-pdf/internal/syntax"
)

type inherited struct {
	resources syntax.Dict
	media     Box
	crop      Box
	rotate    int
	userUnit  float64
	haveMedia bool
	haveCrop  bool
	haveRot   bool
	haveUnit  bool
}

type frame struct {
	node  syntax.Dict
	ref   syntax.Ref
	inh   inherited
	depth int
}

func (d *Document) loadPages(ctx context.Context) error {
	root := d.catalog.Get("Pages")
	if _, ok := root.(syntax.Null); ok {
		return errs.Wrap(errs.ErrCorrupt, "DOC-NO-PAGES", "load page tree", -1, 0, -1)
	}
	stack := []frame{{inh: inherited{}}}
	first := true
	visited := map[syntax.Ref]bool{}
	for len(stack) > 0 {
		fr := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if fr.depth > d.budget().MaxPageTreeDepth {
			return errs.Wrap(errs.ErrBudget, "BUD-PAGE-TREE-DEPTH", "load page tree", -1, 0, -1)
		}
		node := fr.node
		ref := fr.ref
		if first {
			first = false
			o, r, err := d.resolveNode(ctx, root)
			if err != nil {
				return err
			}
			node, ref = o, r
		}
		if ref.Num != 0 {
			if visited[ref] {
				if !d.opts.Repair {
					return errs.Wrap(errs.ErrCorrupt, "DOC-PAGE-TREE-CYCLE", "load page tree", -1, ref.Num, -1)
				}
				d.file.Diag().Add(diag.Diagnostic{Level: diag.Warn, Code: "DOC-PAGE-TREE-CYCLE", Msg: "skipped repeated page tree node", Page: -1, Obj: ref.Num, Offset: -1})
				continue
			}
			visited[ref] = true
		}
		inh, err := d.applyInherited(ctx, fr.inh, node)
		if err != nil {
			return err
		}
		typ, _ := node.NameValue("Type")
		kids, hasKids := node.ArrayValue("Kids")
		if typ == "Page" || (typ == "" && !hasKids) {
			if typ == "" {
				d.file.Diag().Add(diag.Diagnostic{Level: diag.Warn, Code: "DOC-PAGE-TYPE", Msg: "page node missing Type", Page: -1, Offset: -1})
			}
			p, err := d.makePage(ctx, len(d.pages), node, inh)
			if err != nil {
				return err
			}
			d.pages = append(d.pages, p)
			continue
		}
		if typ != "" && typ != "Pages" {
			d.file.Diag().Add(diag.Diagnostic{Level: diag.Warn, Code: "DOC-PAGES-TYPE", Msg: "unexpected page tree node type", Page: -1, Offset: -1})
		}
		for i := len(kids) - 1; i >= 0; i-- {
			o, r, err := d.resolveNode(ctx, kids[i])
			if err != nil {
				return err
			}
			stack = append(stack, frame{node: o, ref: r, inh: inh, depth: fr.depth + 1})
		}
	}
	if len(d.pages) == 0 {
		d.file.Diag().Add(diag.Diagnostic{Level: diag.Warn, Code: "DOC-NO-PAGES", Msg: "page tree contains no pages", Page: -1, Offset: -1})
	}
	return nil
}

func (d *Document) budget() *budget.Budget {
	if d.opts.Tracker != nil {
		return d.opts.Tracker.Budget()
	}
	return &budget.DefaultBudget
}

func (d *Document) resolveNode(ctx context.Context, o syntax.Object) (syntax.Dict, syntax.Ref, error) {
	var ref syntax.Ref
	if r, ok := o.(syntax.Ref); ok {
		ref = r
		v, err := d.file.Resolve(ctx, r)
		if err != nil {
			return nil, ref, err
		}
		o = v
	}
	dict, ok := o.(syntax.Dict)
	if !ok {
		return nil, ref, errs.Wrap(errs.ErrCorrupt, "DOC-NODE-TYPE", "load page tree", -1, ref.Num, -1)
	}
	return dict, ref, nil
}

func (d *Document) applyInherited(ctx context.Context, in inherited, node syntax.Dict) (inherited, error) {
	out := in
	if r, ok := node.DictValue("Resources"); ok {
		out.resources = r
	} else if v, ok := node["Resources"]; ok {
		if rr, ok := v.(syntax.Ref); ok {
			o, err := d.file.Resolve(ctx, rr)
			if err != nil {
				return out, err
			}
			if dict, ok := o.(syntax.Dict); ok {
				out.resources = dict
			}
		}
	}
	if v, ok := node["MediaBox"]; ok {
		b, err := d.boxFrom(ctx, v)
		if err != nil {
			return out, err
		}
		out.media, out.haveMedia = b, true
	}
	if v, ok := node["CropBox"]; ok {
		b, err := d.boxFrom(ctx, v)
		if err != nil {
			return out, err
		}
		out.crop, out.haveCrop = b, true
	}
	if v, ok := node["Rotate"]; ok {
		n, err := d.number(ctx, v)
		if err != nil {
			return out, err
		}
		out.rotate, out.haveRot = normalizeRotate(int(n)), true
	}
	if v, ok := node["UserUnit"]; ok {
		n, err := d.number(ctx, v)
		if err == nil && n > 0 {
			out.userUnit, out.haveUnit = n, true
		}
	}
	return out, nil
}

func (d *Document) makePage(ctx context.Context, index int, node syntax.Dict, inh inherited) (*Page, error) {
	media := inh.media
	if !inh.haveMedia {
		media = Box{0, 0, 612, 792}
		d.file.Diag().Add(diag.Diagnostic{Level: diag.Warn, Code: "DOC-NO-MEDIABOX", Msg: "page has no MediaBox, using Letter", Page: index, Offset: -1})
	}
	crop := media
	if inh.haveCrop {
		crop = inh.crop
	}
	unit := 1.0
	if inh.haveUnit {
		unit = inh.userUnit
	}
	rotate := 0
	if inh.haveRot {
		rotate = inh.rotate
	}
	w, h := crop.Width(), crop.Height()
	if w < 0 {
		w = -w
	}
	if h < 0 {
		h = -h
	}
	if rotate == 90 || rotate == 270 {
		w, h = h, w
	}
	p := &Page{
		doc: d, index: index, dict: node, resources: inh.resources,
		media: media, crop: crop, rotate: rotate, userUnit: unit,
		width: w * unit, height: h * unit,
	}
	p.matrix = pageMatrix(crop, rotate, unit)
	return p, nil
}

func normalizeRotate(r int) int {
	r %= 360
	if r < 0 {
		r += 360
	}
	switch r {
	case 90, 180, 270:
		return r
	}
	return 0
}

// pageMatrix maps PDF user space (origin bottom-left) to page space (origin
// top-left, Y down) after applying CropBox, rotation and UserUnit.
func pageMatrix(c Box, rotate int, s float64) [6]float64 {
	x0, y0, x1, y1 := c.X0, c.Y0, c.X1, c.Y1
	switch rotate {
	case 90:
		return [6]float64{0, s, s, 0, -y0 * s, -x0 * s}
	case 180:
		return [6]float64{-s, 0, 0, s, x1 * s, -y0 * s}
	case 270:
		return [6]float64{0, -s, -s, 0, y1 * s, x1 * s}
	default:
		return [6]float64{s, 0, 0, -s, -x0 * s, y1 * s}
	}
}

func (d *Document) boxFrom(ctx context.Context, o syntax.Object) (Box, error) {
	o, err := d.file.Resolve(ctx, o)
	if err != nil {
		return Box{}, err
	}
	arr, ok := o.(syntax.Array)
	if !ok || len(arr) < 4 {
		return Box{}, errs.Wrap(errs.ErrCorrupt, "DOC-BOX", "read page box", -1, 0, -1)
	}
	var v [4]float64
	for i := 0; i < 4; i++ {
		n, err := d.number(ctx, arr[i])
		if err != nil {
			return Box{}, err
		}
		v[i] = n
	}
	return Box{X0: v[0], Y0: v[1], X1: v[2], Y1: v[3]}, nil
}

func (d *Document) number(ctx context.Context, o syntax.Object) (float64, error) {
	o, err := d.file.Resolve(ctx, o)
	if err != nil {
		return 0, err
	}
	n, ok := syntax.AsNumber(o)
	if !ok {
		return 0, errs.Wrap(errs.ErrCorrupt, "DOC-NUMBER", "read number", -1, 0, -1)
	}
	return n, nil
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

func (p *Page) Index() int { return p.index }

func (p *Page) Size() (w, h float64) { return p.width, p.height }

func (p *Page) Info() PageInfo {
	return PageInfo{
		Index: p.index, Width: p.width, Height: p.height, Rotate: p.rotate,
		UserUnit: p.userUnit, MediaBox: p.media, CropBox: p.crop, Matrix: p.matrix,
	}
}

func (p *Page) Dict() syntax.Dict      { return p.dict }
func (p *Page) Resources() syntax.Dict { return p.resources }
func (p *Page) Document() *Document    { return p.doc }

// Content returns the concatenated, decoded page content stream(s).
func (p *Page) Content(ctx context.Context) ([]byte, error) {
	v, ok := p.dict["Contents"]
	if !ok {
		return nil, nil
	}
	var out []byte
	if err := p.appendContent(ctx, v, &out); err != nil {
		return nil, err
	}
	limit := int64(64 << 20)
	if p.doc.opts.Tracker != nil {
		limit = p.doc.opts.Tracker.Budget().MaxStreamBytes
	}
	if int64(len(out)) > limit {
		return nil, errs.Wrap(errs.ErrBudget, "BUD-STREAM-BYTES", "read page contents", -1, 0, p.index)
	}
	return out, nil
}

func (p *Page) appendContent(ctx context.Context, o syntax.Object, out *[]byte) error {
	o, err := p.doc.file.Resolve(ctx, o)
	if err != nil {
		return err
	}
	switch v := o.(type) {
	case *syntax.Stream:
		data, err := p.doc.file.DecodeStream(ctx, v)
		if err != nil {
			return err
		}
		*out = append(*out, data...)
		*out = append(*out, '\n')
	case syntax.Array:
		for _, item := range v {
			if err := p.appendContent(ctx, item, out); err != nil {
				return err
			}
		}
	}
	return nil
}

// Interpret runs the G1 content interpreter and streams display items.
func (p *Page) Interpret(ctx context.Context, emit func(displaylist.Item) error) (*content.Result, error) {
	p.doc.mu.RLock()
	closed := p.doc.closed
	p.doc.mu.RUnlock()
	if closed {
		return nil, errs.Wrap(errs.ErrClosed, "DOC-CLOSED", "interpret page", -1, 0, p.index)
	}
	data, err := p.Content(ctx)
	if err != nil {
		return nil, err
	}
	info := p.Info()
	tracker := p.doc.opts.Tracker
	if tracker == nil {
		tracker = budget.NewTracker(&budget.DefaultBudget)
	}
	dc := p.doc.opts.Diag
	if dc == nil {
		dc = p.doc.file.Diag()
	}
	return content.Interpret(ctx, content.PageData{
		Index: p.index, Content: data, Resources: p.resources,
		Matrix:   info.Matrix,
		MediaBox: displaylist.Rect{X0: info.MediaBox.X0, Y0: info.MediaBox.Y0, X1: info.MediaBox.X1, Y1: info.MediaBox.Y1},
		CropBox:  displaylist.Rect{X0: info.CropBox.X0, Y0: info.CropBox.Y0, X1: info.CropBox.X1, Y1: info.CropBox.Y1},
		File:     p.doc.file, Fonts: p.doc.fonts, Tracker: tracker, Diag: dc, OC: p.doc.oc,
		PreserveForms: p.doc.opts.PreserveForms,
	}, emit)
}
