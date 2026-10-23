package doc

import (
	"context"

	"github.com/F31/go-pdf/internal/syntax"
)

// StructureItem is a lightweight tagged-PDF structure tree node.
type StructureItem struct {
	ID       int
	Type     string
	Title    string
	Page     int
	MCIDs    []int
	Children []StructureItem
}

func (d *Document) StructureTree(ctx context.Context) ([]StructureItem, error) {
	obj, err := d.file.Resolve(ctx, d.catalog.Get("StructTreeRoot"))
	if err != nil {
		return nil, nil
	}
	root, ok := obj.(syntax.Dict)
	if !ok {
		return nil, nil
	}
	seen := map[syntax.Ref]bool{}
	kids := root.Get("K")
	return d.structureNodes(ctx, kids, -1, seen, 0)
}

func (d *Document) structureNodes(ctx context.Context, obj syntax.Object, inheritedPage int, seen map[syntax.Ref]bool, depth int) ([]StructureItem, error) {
	if obj == nil || depth > 32 {
		return nil, nil
	}
	if m, ok := obj.(syntax.Int); ok {
		return []StructureItem{{Page: inheritedPage, MCIDs: []int{int(m)}}}, nil
	}
	id := 0
	if r, ok := obj.(syntax.Ref); ok {
		id = r.Num
	}
	if arr, ok := obj.(syntax.Array); ok {
		var out []StructureItem
		for _, item := range arr {
			nodes, err := d.structureNodes(ctx, item, inheritedPage, seen, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, nodes...)
		}
		return out, nil
	}
	if r, ok := obj.(syntax.Ref); ok {
		if seen[r] {
			return nil, nil
		}
		seen[r] = true
	}
	resolved, err := d.file.Resolve(ctx, obj)
	if err != nil {
		return nil, nil
	}
	dict, ok := resolved.(syntax.Dict)
	if !ok {
		return nil, nil
	}
	typ, _ := dict.NameValue("Type")
	if typ == "MCR" {
		page := inheritedPage
		if v, ok := dict.Get("Pg").(syntax.Ref); ok {
			page = d.pageIndexRef(v)
		}
		if m, ok := dict.IntValue("MCID"); ok {
			return []StructureItem{{ID: id, Page: page, MCIDs: []int{int(m)}}}, nil
		}
		return nil, nil
	}
	if typ != "StructElem" && typ != "" {
		return nil, nil
	}
	page := inheritedPage
	if v, ok := dict.Get("Pg").(syntax.Ref); ok {
		page = d.pageIndexRef(v)
	}
	node := StructureItem{ID: id, Type: nameValue(dict, "S"), Title: textValue(dict.Get("T")), Page: page}
	if m, ok := dict.IntValue("MCID"); ok {
		node.MCIDs = append(node.MCIDs, int(m))
	}
	if k := dict.Get("K"); k != nil {
		kids, err := d.structureNodes(ctx, k, page, seen, depth+1)
		if err != nil {
			return nil, err
		}
		for _, kid := range kids {
			if kid.Type == "" && len(kid.Children) == 0 {
				node.MCIDs = append(node.MCIDs, kid.MCIDs...)
				continue
			}
			node.Children = append(node.Children, kid)
		}
	}
	return []StructureItem{node}, nil
}

func nameValue(d syntax.Dict, name string) string {
	v, _ := d.NameValue(name)
	return v
}

func textValue(o syntax.Object) string {
	if s, ok := o.(syntax.String); ok {
		return decodeBytes(s)
	}
	return ""
}
