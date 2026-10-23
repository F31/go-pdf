package pdf

import (
	"context"

	"github.com/F31/go-pdf/internal/doc"
)

// StructElement is a lightweight tagged-PDF structure tree node.
type StructElement struct {
	ID       int
	Type     string
	Title    string
	Page     int
	MCIDs    []int
	Children []StructElement
}

// StructureTree returns the tagged-PDF structure tree when present.
func (d *Document) StructureTree(ctx context.Context) ([]StructElement, error) {
	if d == nil || d.inner == nil {
		return nil, nil
	}
	d.structureMu.Lock()
	defer d.structureMu.Unlock()
	if d.structureDone {
		return d.structure, d.structureErr
	}
	src, err := d.inner.StructureTree(ctx)
	if err != nil {
		d.structureErr = err
		return nil, err
	}
	d.structure = convertStructure(src)
	d.structureDone = true
	return d.structure, nil
}

func convertStructure(src []doc.StructureItem) []StructElement {
	out := make([]StructElement, len(src))
	for i, s := range src {
		out[i] = StructElement{
			ID: s.ID, Type: s.Type, Title: s.Title, Page: s.Page,
			MCIDs: append([]int(nil), s.MCIDs...), Children: convertStructure(s.Children),
		}
	}
	return out
}
