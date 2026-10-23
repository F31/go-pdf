package doc

import (
	"context"

	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/internal/syntax"
)

// FormFieldItem is an internal AcroForm field tree node.
type FormFieldItem struct {
	Name     string
	Type     string
	Value    string
	Page     int
	Rect     displaylist.Rect
	Flags    int
	Children []FormFieldItem
}

func (d *Document) FormFields(ctx context.Context) ([]FormFieldItem, error) {
	obj, err := d.file.Resolve(ctx, d.catalog.Get("AcroForm"))
	if err != nil {
		return nil, nil
	}
	form, ok := obj.(syntax.Dict)
	if !ok {
		return nil, nil
	}
	fields, ok := form.Get("Fields").(syntax.Array)
	if !ok {
		return nil, nil
	}
	var out []FormFieldItem
	for _, f := range fields {
		if item, ok := d.formField(ctx, f, "", "", -1); ok {
			out = append(out, item)
		}
	}
	return out, nil
}

func (d *Document) formField(ctx context.Context, obj syntax.Object, prefix, inheritedType string, inheritedPage int) (FormFieldItem, bool) {
	resolved, err := d.file.Resolve(ctx, obj)
	if err != nil {
		return FormFieldItem{}, false
	}
	dict, ok := resolved.(syntax.Dict)
	if !ok {
		return FormFieldItem{}, false
	}
	name := prefix
	if v, ok := dict.Get("T").(syntax.String); ok {
		part := decodeTextString(v)
		if name != "" && part != "" {
			name += "."
		}
		name += part
	}
	typ := inheritedType
	if v, ok := dict.NameValue("FT"); ok {
		typ = v
	}
	page := inheritedPage
	if v, ok := dict.Get("P").(syntax.Ref); ok {
		page = d.pageIndexRef(v)
	}
	rect := displaylist.Rect{}
	if r, ok := d.annotationRect(ctx, dict.Get("Rect"), page); ok {
		rect = r
	}
	item := FormFieldItem{Name: name, Type: typ, Value: valueString(dict.Get("V")), Page: page, Rect: rect}
	if f, ok := dict.IntValue("Ff"); ok {
		item.Flags = int(f)
	}
	if kids, ok := dict.Get("Kids").(syntax.Array); ok {
		for _, kid := range kids {
			kobj, err := d.file.Resolve(ctx, kid)
			if err != nil {
				continue
			}
			kd, ok := kobj.(syntax.Dict)
			if !ok {
				continue
			}
			if sub, _ := kd.NameValue("Subtype"); sub == "Widget" {
				kpage := page
				if v, ok := kd.Get("P").(syntax.Ref); ok {
					kpage = d.pageIndexRef(v)
				}
				if r, ok := d.annotationRect(ctx, kd.Get("Rect"), kpage); ok {
					item.Rect = r
					item.Page = kpage
				}
				continue
			}
			if child, ok := d.formField(ctx, kid, name, typ, page); ok {
				item.Children = append(item.Children, child)
			}
		}
	}
	return item, true
}

func (d *Document) pageIndexRef(ref syntax.Ref) int {
	for i, p := range d.pages {
		if p.ref == ref {
			return i
		}
	}
	return -1
}

func (d *Document) annotationRect(ctx context.Context, o syntax.Object, page int) (displaylist.Rect, bool) {
	if o == nil || page < 0 || page >= len(d.pages) {
		return displaylist.Rect{}, false
	}
	p := d.pages[page]
	return p.linkRect(ctx, o)
}
