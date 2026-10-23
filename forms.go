package pdf

import (
	"context"

	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/internal/doc"
)

// FormField is an AcroForm field tree node.
type FormField struct {
	Name     string
	Type     string
	Value    string
	Page     int
	Rect     displaylist.Rect
	Flags    int
	Children []FormField
}

// FormFields returns the AcroForm field tree.
func (d *Document) FormFields(ctx context.Context) ([]FormField, error) {
	if d == nil || d.inner == nil {
		return nil, nil
	}
	src, err := d.inner.FormFields(ctx)
	if err != nil {
		return nil, err
	}
	return convertFields(src), nil
}

func convertFields(src []doc.FormFieldItem) []FormField {
	out := make([]FormField, len(src))
	for i, f := range src {
		out[i] = FormField{
			Name: f.Name, Type: f.Type, Value: f.Value, Page: f.Page,
			Rect: f.Rect, Flags: f.Flags, Children: convertFields(f.Children),
		}
	}
	return out
}
