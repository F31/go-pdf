package doc

import (
	"context"
	"strings"

	"github.com/F31/go-pdf/internal/syntax"
)

// OutlineItem is the internal outline tree node.
type OutlineItem struct {
	Title    string
	Page     int
	URI      string
	Children []OutlineItem
}

func (d *Document) Outlines(ctx context.Context) ([]OutlineItem, error) {
	obj, err := d.file.Resolve(ctx, d.catalog.Get("Outlines"))
	if err != nil {
		return nil, nil
	}
	root, ok := obj.(syntax.Dict)
	if !ok {
		return nil, nil
	}
	seen := map[syntax.Ref]bool{}
	first := root.Get("First")
	items, err := d.outlineSiblings(ctx, first, seen, 0)
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (d *Document) outlineSiblings(ctx context.Context, obj syntax.Object, seen map[syntax.Ref]bool, depth int) ([]OutlineItem, error) {
	if depth > 16 {
		return nil, nil
	}
	var out []OutlineItem
	for obj != nil {
		var ref syntax.Ref
		if r, ok := obj.(syntax.Ref); ok {
			if seen[r] {
				break
			}
			seen[r] = true
			ref = r
		}
		resolved, err := d.file.Resolve(ctx, obj)
		if err != nil {
			break
		}
		dict, ok := resolved.(syntax.Dict)
		if !ok {
			break
		}
		item := OutlineItem{Title: decodeTextString(dict.Get("Title")), Page: -1}
		if v := dict.Get("Dest"); v != nil {
			item.Page = d.destPage(ctx, v, 0)
		}
		if a := dict.Get("A"); a != nil {
			if av, err := d.file.Resolve(ctx, a); err == nil {
				if action, ok := av.(syntax.Dict); ok {
					if s, _ := action.NameValue("S"); s == "URI" {
						if u, ok := action.Get("URI").(syntax.String); ok {
							item.URI = string(u)
						}
					} else if s == "GoTo" {
						item.Page = d.destPage(ctx, action.Get("D"), 0)
					}
				}
			}
		}
		if first := dict.Get("First"); first != nil {
			children, err := d.outlineSiblings(ctx, first, seen, depth+1)
			if err != nil {
				return nil, err
			}
			item.Children = children
		}
		out = append(out, item)
		next := dict.Get("Next")
		if next == nil {
			break
		}
		if r, ok := next.(syntax.Ref); ok && ref.Num != 0 && r == ref {
			break
		}
		obj = next
	}
	return out, nil
}

func (d *Document) namedDest(ctx context.Context, name string) syntax.Object {
	obj, err := d.file.Resolve(ctx, d.catalog.Get("Dests"))
	if err == nil {
		if dict, ok := obj.(syntax.Dict); ok {
			if v := dict.Get(name); v != nil {
				return v
			}
		}
	}
	names, err := d.file.Resolve(ctx, d.catalog.Get("Names"))
	if err != nil {
		return nil
	}
	dict, ok := names.(syntax.Dict)
	if !ok {
		return nil
	}
	dests, err := d.file.Resolve(ctx, dict.Get("Dests"))
	if err != nil {
		return nil
	}
	return findNameTree(ctx, d, dests, name, 0)
}

func findNameTree(ctx context.Context, d *Document, obj syntax.Object, name string, depth int) syntax.Object {
	if depth > 16 {
		return nil
	}
	obj, err := d.file.Resolve(ctx, obj)
	if err != nil {
		return nil
	}
	dict, ok := obj.(syntax.Dict)
	if !ok {
		return nil
	}
	if arr, ok := dict.Get("Names").(syntax.Array); ok {
		for i := 0; i+1 < len(arr); i += 2 {
			k, ok := arr[i].(syntax.String)
			if ok && string(k) == name {
				return arr[i+1]
			}
		}
	}
	if kids, ok := dict.Get("Kids").(syntax.Array); ok {
		for _, kid := range kids {
			if v := findNameTree(ctx, d, kid, name, depth+1); v != nil {
				return v
			}
		}
	}
	return nil
}

func decodeTextString(o syntax.Object) string {
	s, ok := o.(syntax.String)
	if !ok {
		return ""
	}
	return decodeString([]byte(s))
}

func utf16BE(b []byte) string {
	var sb strings.Builder
	for i := 0; i+1 < len(b); i += 2 {
		r := rune(b[i])<<8 | rune(b[i+1])
		if r >= 0xD800 && r <= 0xDBFF && i+3 < len(b) {
			r2 := rune(b[i+2])<<8 | rune(b[i+3])
			r = 0x10000 + (r-0xD800)<<10 + (r2 - 0xDC00)
			i += 2
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

func utf16LE(b []byte) string {
	var sb strings.Builder
	for i := 0; i+1 < len(b); i += 2 {
		r := rune(b[i+1])<<8 | rune(b[i])
		if r >= 0xD800 && r <= 0xDBFF && i+3 < len(b) {
			r2 := rune(b[i+3])<<8 | rune(b[i+2])
			r = 0x10000 + (r-0xD800)<<10 + (r2 - 0xDC00)
			i += 2
		}
		sb.WriteRune(r)
	}
	return sb.String()
}
