package pdf

import (
	"context"

	"github.com/F31/go-pdf/internal/doc"
)

// Outline is a document outline entry. Page is zero-based and is -1 when the
// destination cannot be resolved. URI is set for URI actions.
type Outline struct {
	Title    string
	Page     int
	URI      string
	Children []Outline
}

// Outlines returns the document outline tree.
func (d *Document) Outlines(ctx context.Context) ([]Outline, error) {
	if d == nil || d.inner == nil {
		return nil, nil
	}
	src, err := d.inner.Outlines(ctx)
	if err != nil {
		return nil, err
	}
	return convertOutlines(src), nil
}

func convertOutlines(src []doc.OutlineItem) []Outline {
	out := make([]Outline, len(src))
	for i, item := range src {
		out[i] = Outline{Title: item.Title, Page: item.Page, URI: item.URI, Children: convertOutlines(item.Children)}
	}
	return out
}
