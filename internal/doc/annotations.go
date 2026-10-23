package doc

import (
	"context"
	"math"
	"strconv"

	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/internal/content"
	"github.com/F31/go-pdf/internal/syntax"
)

// Annotations returns generic annotation and widget metadata.
func (p *Page) Annotations(ctx context.Context) ([]displaylist.Annotation, error) {
	annots, ok := p.dict.Get("Annots").(syntax.Array)
	if !ok {
		return nil, nil
	}
	var out []displaylist.Annotation
	for _, item := range annots {
		obj, err := p.doc.file.Resolve(ctx, item)
		if err != nil {
			continue
		}
		dict, ok := obj.(syntax.Dict)
		if !ok {
			continue
		}
		sub, _ := dict.NameValue("Subtype")
		rect, ok := p.linkRect(ctx, dict.Get("Rect"))
		if !ok {
			continue
		}
		a := displaylist.Annotation{Subtype: sub, Rect: rect, Page: -1}
		if v, ok := dict.Get("Contents").(syntax.String); ok {
			a.Contents = decodeTextString(v)
		}
		if v, ok := dict.IntValue("F"); ok {
			a.Flags = int(v)
		}
		if v, ok := dict.Get("T").(syntax.String); ok {
			a.FieldName = decodeTextString(v)
		}
		if v, ok := dict.NameValue("FT"); ok {
			a.FieldType = v
		}
		a.FieldValue = valueString(dict.Get("V"))
		if v := dict.Get("A"); v != nil {
			if av, err := p.doc.file.Resolve(ctx, v); err == nil {
				if action, ok := av.(syntax.Dict); ok {
					if s, _ := action.NameValue("S"); s == "URI" {
						if u, ok := action.Get("URI").(syntax.String); ok {
							a.URI = string(u)
						}
					} else if s == "GoTo" {
						a.Page = p.doc.destPage(ctx, action.Get("D"), 0)
					}
				}
			}
		}
		if v := dict.Get("Dest"); v != nil {
			a.Page = p.doc.destPage(ctx, v, 0)
		}
		if appearance, ok := p.annotationAppearance(ctx, dict, rect); ok {
			a.Appearance = appearance
		}
		out = append(out, a)
	}
	return out, nil
}

func valueString(o syntax.Object) string {
	switch v := o.(type) {
	case syntax.String:
		return decodeBytes(v)
	case syntax.Name:
		return string(v)
	case syntax.Int:
		return strconv.FormatInt(int64(v), 10)
	case syntax.Real:
		return strconv.FormatFloat(float64(v), 'g', -1, 64)
	case syntax.Bool:
		if v {
			return "true"
		}
		return "false"
	}
	return ""
}

func (p *Page) annotationAppearance(ctx context.Context, annot syntax.Dict, rect displaylist.Rect) (*displaylist.AnnotationAppearance, bool) {
	apObj, err := p.doc.file.Resolve(ctx, annot.Get("AP"))
	if err != nil {
		return nil, false
	}
	ap, ok := apObj.(syntax.Dict)
	if !ok {
		return nil, false
	}
	nObj, err := p.doc.file.Resolve(ctx, ap.Get("N"))
	if err != nil {
		return nil, false
	}
	if states, ok := nObj.(syntax.Dict); ok {
		var state syntax.Name
		if s, ok := annot.NameValue("AS"); ok {
			state = syntax.Name(s)
		}
		nObj = states.Get(string(state))
		if nObj == nil {
			for _, v := range states {
				nObj = v
				break
			}
		}
		if nObj == nil {
			return nil, false
		}
		nObj, err = p.doc.file.Resolve(ctx, nObj)
		if err != nil {
			return nil, false
		}
	}
	stm, ok := nObj.(*syntax.Stream)
	if !ok {
		return nil, false
	}
	bbox, ok := p.appearanceBox(ctx, stm.Dict.Get("BBox"))
	if !ok || bbox.X1 <= bbox.X0 || bbox.Y1 <= bbox.Y0 {
		return nil, false
	}
	apm := [6]float64{1, 0, 0, 1, 0, 0}
	if v := stm.Dict.Get("Matrix"); v != nil {
		if m, ok := p.matrixValue(ctx, v); ok {
			apm = m
		}
	}
	p0 := applyMatrix(apm, bbox.X0, bbox.Y0)
	p1 := applyMatrix(apm, bbox.X1, bbox.Y0)
	p2 := applyMatrix(apm, bbox.X1, bbox.Y1)
	p3 := applyMatrix(apm, bbox.X0, bbox.Y1)
	bbox = displaylist.Rect{
		X0: math.Min(math.Min(p0[0], p1[0]), math.Min(p2[0], p3[0])),
		Y0: math.Min(math.Min(p0[1], p1[1]), math.Min(p2[1], p3[1])),
		X1: math.Max(math.Max(p0[0], p1[0]), math.Max(p2[0], p3[0])),
		Y1: math.Max(math.Max(p0[1], p1[1]), math.Max(p2[1], p3[1])),
	}
	res := p.resources
	if stm.Dict.Has("Resources") {
		if d, err := p.doc.file.Resolve(ctx, stm.Dict.Get("Resources")); err == nil {
			if rd, ok := d.(syntax.Dict); ok {
				res = rd
			}
		}
	}
	data, err := p.doc.file.DecodeStream(ctx, stm)
	if err != nil {
		return nil, false
	}
	sx := (rect.X1 - rect.X0) / (bbox.X1 - bbox.X0)
	sy := (rect.Y1 - rect.Y0) / (bbox.Y1 - bbox.Y0)
	place := [6]float64{sx, 0, 0, sy, rect.X0 - bbox.X0*sx, rect.Y0 - bbox.Y0*sy}
	ctm := mulMatrix(place, apm)
	var items []displaylist.Item
	resp, err := content.Interpret(ctx, content.PageData{
		Index: p.index, Content: data, Resources: res, Matrix: ctm,
		MediaBox: displaylist.Rect{X0: p.media.X0, Y0: p.media.Y0, X1: p.media.X1, Y1: p.media.Y1},
		CropBox:  displaylist.Rect{X0: p.crop.X0, Y0: p.crop.Y0, X1: p.crop.X1, Y1: p.crop.Y1},
		File:     p.doc.file, Fonts: p.doc.fonts, Tracker: p.doc.opts.Tracker,
		Diag: p.doc.opts.Diag, OC: p.doc.oc, PreserveForms: p.doc.opts.PreserveForms,
		Lease:    p.doc.acquireLease,
		Decoders: p.doc.opts.Decoders, ColorManager: p.doc.opts.ColorManager,
	}, func(item displaylist.Item) error {
		items = append(items, item)
		return nil
	})
	if err != nil {
		return nil, false
	}
	return &displaylist.AnnotationAppearance{Items: items, Clips: resp.Clips}, true
}

func (p *Page) appearanceBox(ctx context.Context, o syntax.Object) (displaylist.Rect, bool) {
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
	return displaylist.Rect{X0: math.Min(v[0], v[2]), Y0: math.Min(v[1], v[3]), X1: math.Max(v[0], v[2]), Y1: math.Max(v[1], v[3])}, true
}

func (p *Page) matrixValue(ctx context.Context, o syntax.Object) ([6]float64, bool) {
	obj, err := p.doc.file.Resolve(ctx, o)
	if err != nil {
		return [6]float64{}, false
	}
	arr, ok := obj.(syntax.Array)
	if !ok || len(arr) < 6 {
		return [6]float64{}, false
	}
	var m [6]float64
	for i := range m {
		n, err := p.doc.number(ctx, arr[i])
		if err != nil {
			return [6]float64{}, false
		}
		m[i] = n
	}
	return m, true
}

func mulMatrix(a, b [6]float64) [6]float64 {
	return [6]float64{
		a[0]*b[0] + a[2]*b[1], a[1]*b[0] + a[3]*b[1],
		a[0]*b[2] + a[2]*b[3], a[1]*b[2] + a[3]*b[3],
		a[0]*b[4] + a[2]*b[5] + a[4], a[1]*b[4] + a[3]*b[5] + a[5],
	}
}
