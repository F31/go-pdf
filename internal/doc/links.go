package doc

import (
	"context"

	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/internal/syntax"
)

// Links resolves link annotations on this page.
func (p *Page) Links(ctx context.Context) ([]displaylist.Link, error) {
	annots, ok := p.dict.Get("Annots").(syntax.Array)
	if !ok {
		return nil, nil
	}
	var out []displaylist.Link
	for _, item := range annots {
		obj, err := p.doc.file.Resolve(ctx, item)
		if err != nil {
			continue
		}
		annot, ok := obj.(syntax.Dict)
		if !ok {
			continue
		}
		if sub, _ := annot.NameValue("Subtype"); sub != "Link" {
			continue
		}
		rect, ok := p.linkRect(ctx, annot.Get("Rect"))
		if !ok {
			continue
		}
		link := displaylist.Link{Rect: rect, Page: -1}
		if a := annot.Get("A"); a != nil {
			if obj, err := p.doc.file.Resolve(ctx, a); err == nil {
				if action, ok := obj.(syntax.Dict); ok {
					if s, _ := action.NameValue("S"); s == "URI" {
						if u, ok := action.Get("URI").(syntax.String); ok {
							link.URI = string(u)
						}
					} else if s == "GoTo" {
						link.Page = p.linkDest(ctx, action.Get("D"))
					}
				}
			}
		}
		if d := annot.Get("Dest"); d != nil {
			link.Page = p.linkDest(ctx, d)
		}
		if link.URI == "" && link.Page < 0 {
			continue
		}
		out = append(out, link)
	}
	return out, nil
}

func (p *Page) linkRect(ctx context.Context, o syntax.Object) (displaylist.Rect, bool) {
	obj, err := p.doc.file.Resolve(ctx, o)
	if err != nil {
		return displaylist.Rect{}, false
	}
	arr, ok := obj.(syntax.Array)
	if !ok || len(arr) < 4 {
		return displaylist.Rect{}, false
	}
	var v [4]float64
	for i := range v {
		n, err := p.doc.number(ctx, arr[i])
		if err != nil {
			return displaylist.Rect{}, false
		}
		v[i] = n
	}
	p0 := applyMatrix(p.matrix, v[0], v[1])
	p1 := applyMatrix(p.matrix, v[2], v[3])
	return displaylist.Rect{
		X0: min(p0[0], p1[0]), Y0: min(p0[1], p1[1]),
		X1: max(p0[0], p1[0]), Y1: max(p0[1], p1[1]),
	}, true
}

func (p *Page) linkDest(ctx context.Context, o syntax.Object) int {
	return p.doc.destPage(ctx, o, 0)
}

func (d *Document) destPage(ctx context.Context, o syntax.Object, depth int) int {
	if o == nil || depth > 8 {
		return -1
	}
	obj, err := d.file.Resolve(ctx, o)
	if err != nil {
		return -1
	}
	switch v := obj.(type) {
	case syntax.Name:
		return d.destPage(ctx, d.namedDest(ctx, string(v)), depth+1)
	case syntax.String:
		return d.destPage(ctx, d.namedDest(ctx, string(v)), depth+1)
	case syntax.Dict:
		if v.Has("D") {
			return d.destPage(ctx, v.Get("D"), depth+1)
		}
	case syntax.Array:
		if len(v) == 0 {
			return -1
		}
		ref, ok := v[0].(syntax.Ref)
		if !ok {
			return -1
		}
		for i, page := range d.pages {
			if page.ref == ref {
				return i
			}
		}
	}
	return -1
}

func applyMatrix(m [6]float64, x, y float64) [2]float64 {
	return [2]float64{m[0]*x + m[2]*y + m[4], m[1]*x + m[3]*y + m[5]}
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
