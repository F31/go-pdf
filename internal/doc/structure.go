package doc

import (
	"context"

	"github.com/F31/go-pdf/internal/syntax"
)

// StructureItem is a lightweight tagged-PDF structure tree node.
type StructureItem struct {
	ID              int
	Type            string
	RawType         string
	Title           string
	Alt             string
	ActualText      string
	Lang            string
	IDName          string
	Classes         []string
	Attributes      []StructAttribute
	ClassAttributes []StructAttribute
	Page            int
	MCIDs           []int
	Children        []StructureItem
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
	roleMap := map[string]string{}
	if rm, ok := root.Get("RoleMap").(syntax.Dict); ok {
		for key, value := range rm {
			if mapped, ok := d.nameValue(ctx, value); ok {
				roleMap[string(key)] = mapped
			}
		}
	}
	classMap := map[string]syntax.Object{}
	if cm, ok := root.Get("ClassMap").(syntax.Dict); ok {
		for key, value := range cm {
			classMap[string(key)] = value
		}
	}
	kids := root.Get("K")
	return d.structureNodes(ctx, kids, -1, seen, 0, roleMap, classMap)
}

func (d *Document) structureNodes(ctx context.Context, obj syntax.Object, inheritedPage int, seen map[syntax.Ref]bool, depth int, roleMap map[string]string, classMap map[string]syntax.Object) ([]StructureItem, error) {
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
			nodes, err := d.structureNodes(ctx, item, inheritedPage, seen, depth+1, roleMap, classMap)
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
	rawType := nameValue(dict, "S")
	roleType := rawType
	if mapped, ok := roleMap[rawType]; ok {
		roleType = mapped
	}
	node := StructureItem{
		ID: id, Type: roleType, RawType: rawType, Title: d.textValue(ctx, dict.Get("T")),
		Alt: d.textValue(ctx, dict.Get("Alt")), ActualText: d.textValue(ctx, dict.Get("ActualText")),
		Lang: d.textValue(ctx, dict.Get("Lang")), IDName: d.textValue(ctx, dict.Get("ID")),
		Classes: d.classNames(ctx, dict.Get("C")), Attributes: d.structAttributes(ctx, dict.Get("A")),
		ClassAttributes: d.classAttributes(ctx, dict.Get("C"), classMap), Page: page,
	}
	if m, ok := dict.IntValue("MCID"); ok {
		node.MCIDs = append(node.MCIDs, int(m))
	}
	if k := dict.Get("K"); k != nil {
		kids, err := d.structureNodes(ctx, k, page, seen, depth+1, roleMap, classMap)
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

func (d *Document) nameValue(ctx context.Context, obj syntax.Object) (string, bool) {
	if r, ok := obj.(syntax.Ref); ok {
		resolved, err := d.file.Resolve(ctx, r)
		if err == nil {
			obj = resolved
		}
	}
	name, ok := obj.(syntax.Name)
	return string(name), ok
}

func (d *Document) classAttributes(ctx context.Context, obj syntax.Object, classMap map[string]syntax.Object) []StructAttribute {
	var out []StructAttribute
	for _, name := range d.classNames(ctx, obj) {
		if value, ok := classMap[name]; ok {
			out = append(out, d.structAttributes(ctx, value)...)
		}
	}
	return out
}

func (d *Document) classNames(ctx context.Context, obj syntax.Object) []string {
	if r, ok := obj.(syntax.Ref); ok {
		resolved, err := d.file.Resolve(ctx, r)
		if err == nil {
			obj = resolved
		}
	}
	var out []string
	switch v := obj.(type) {
	case syntax.Name:
		out = append(out, string(v))
	case syntax.Array:
		for _, item := range v {
			if name, ok := d.nameValue(ctx, item); ok {
				out = append(out, name)
			}
		}
	}
	return out
}

func nameValue(d syntax.Dict, name string) string {
	v, _ := d.NameValue(name)
	return v
}

func (d *Document) textValue(ctx context.Context, o syntax.Object) string {
	if s, ok := o.(syntax.String); ok {
		return decodeBytes(s)
	}
	if r, ok := o.(syntax.Ref); ok {
		resolved, err := d.file.Resolve(ctx, r)
		if err == nil {
			if s, ok := resolved.(syntax.String); ok {
				return decodeBytes(s)
			}
		}
	}
	return ""
}

type StructAttribute struct {
	Owner  string
	Values map[string]StructValue
}

type StructValue struct {
	Kind   string
	Text   string
	Number float64
	Bool   bool
	Items  []StructValue
	Fields map[string]StructValue
}

func (d *Document) structAttributes(ctx context.Context, obj syntax.Object) []StructAttribute {
	obj = d.resolveStructureObject(ctx, obj)
	var out []StructAttribute
	switch v := obj.(type) {
	case syntax.Array:
		for _, item := range v {
			out = append(out, d.structAttributes(ctx, item)...)
		}
	case syntax.Dict:
		owner, _ := d.nameValue(ctx, v.Get("O"))
		attr := StructAttribute{Owner: owner, Values: map[string]StructValue{}}
		for key, value := range v {
			if string(key) == "O" {
				continue
			}
			attr.Values[string(key)] = d.structValue(ctx, value, 0)
		}
		if len(attr.Values) > 0 || owner != "" {
			out = append(out, attr)
		}
	}
	return out
}

func (d *Document) structValue(ctx context.Context, obj syntax.Object, depth int) StructValue {
	if depth > 4 {
		return StructValue{Kind: "null"}
	}
	if r, ok := obj.(syntax.Ref); ok {
		resolved, err := d.file.Resolve(ctx, r)
		if err != nil {
			return StructValue{Kind: "null"}
		}
		obj = resolved
	}
	switch v := obj.(type) {
	case syntax.Name:
		return StructValue{Kind: "name", Text: string(v)}
	case syntax.String:
		return StructValue{Kind: "string", Text: decodeBytes(v)}
	case syntax.Int:
		return StructValue{Kind: "number", Number: float64(v)}
	case syntax.Real:
		return StructValue{Kind: "number", Number: float64(v)}
	case syntax.Bool:
		return StructValue{Kind: "boolean", Bool: bool(v)}
	case syntax.Array:
		items := make([]StructValue, 0, len(v))
		for _, item := range v {
			items = append(items, d.structValue(ctx, item, depth+1))
		}
		return StructValue{Kind: "array", Items: items}
	case syntax.Dict:
		fields := make(map[string]StructValue, len(v))
		for key, value := range v {
			fields[string(key)] = d.structValue(ctx, value, depth+1)
		}
		return StructValue{Kind: "dictionary", Fields: fields}
	default:
		return StructValue{Kind: "null"}
	}
}

func (d *Document) resolveStructureObject(ctx context.Context, obj syntax.Object) syntax.Object {
	if r, ok := obj.(syntax.Ref); ok {
		if resolved, err := d.file.Resolve(ctx, r); err == nil {
			return resolved
		}
	}
	return obj
}
