package pdf

import (
	"context"

	"github.com/F31/go-pdf/internal/doc"
)

// Attachment is a document-level embedded file.
type Attachment struct {
	Name        string
	Description string
	MIME        string
	ModDate     string
	Size        int64
	inner       *doc.Attachment
}

// Data returns the decoded embedded file bytes.
func (a *Attachment) Data(ctx context.Context) ([]byte, error) {
	if a == nil || a.inner == nil {
		return nil, nil
	}
	return a.inner.Data(ctx)
}

// Attachments returns files in the EmbeddedFiles name tree.
func (d *Document) Attachments(ctx context.Context) ([]Attachment, error) {
	if d == nil || d.inner == nil {
		return nil, nil
	}
	src, err := d.inner.Attachments(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Attachment, len(src))
	for i := range src {
		out[i] = Attachment{
			Name: src[i].Name, Description: src[i].Description, MIME: src[i].MIME,
			ModDate: src[i].ModDate, Size: src[i].Size, inner: &src[i],
		}
	}
	return out, nil
}
