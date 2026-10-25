package doc

import (
	"context"
	"io"

	"github.com/F31/go-pdf/internal/errs"
	"github.com/F31/go-pdf/internal/syntax"
)

// Attachment is an embedded file document attachment.
type Attachment struct {
	Name        string
	Description string
	MIME        string
	ModDate     string
	Size        int64
	stream      *syntax.Stream
	doc         *Document
}

// Data decodes the embedded file stream.
func (a *Attachment) Data(ctx context.Context) ([]byte, error) {
	if a == nil || a.stream == nil || a.doc == nil {
		return nil, errs.Wrap(errs.ErrCorrupt, "DOC-ATTACHMENT", "read attachment", -1, 0, -1)
	}
	return a.doc.file.DecodeStream(ctx, a.stream)
}

// WriteTo decodes the embedded file stream directly into w.
func (a *Attachment) WriteTo(ctx context.Context, w io.Writer) (int64, error) {
	if a == nil || a.stream == nil || a.doc == nil {
		return 0, errs.Wrap(errs.ErrCorrupt, "DOC-ATTACHMENT", "read attachment", -1, 0, -1)
	}
	return a.doc.file.DecodeStreamTo(ctx, a.stream, w)
}

// Attachments returns the EmbeddedFiles name tree.
func (d *Document) Attachments(ctx context.Context) ([]Attachment, error) {
	names, err := d.file.Resolve(ctx, d.catalog.Get("Names"))
	if err != nil {
		return nil, nil
	}
	dict, ok := names.(syntax.Dict)
	if !ok {
		return nil, nil
	}
	root, err := d.file.Resolve(ctx, dict.Get("EmbeddedFiles"))
	if err != nil {
		return nil, nil
	}
	var out []Attachment
	err = d.walkNameTree(ctx, root, func(name string, value syntax.Object) {
		if a, ok := d.attachment(ctx, name, value); ok {
			out = append(out, a)
		}
	})
	return out, err
}

func (d *Document) walkNameTree(ctx context.Context, obj syntax.Object, fn func(string, syntax.Object)) error {
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
			name, ok := arr[i].(syntax.String)
			if !ok {
				continue
			}
			fn(decodeBytes(name), arr[i+1])
		}
	}
	if kids, ok := dict.Get("Kids").(syntax.Array); ok {
		for _, kid := range kids {
			if err := d.walkNameTree(ctx, kid, fn); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *Document) attachment(ctx context.Context, name string, obj syntax.Object) (Attachment, bool) {
	obj, err := d.file.Resolve(ctx, obj)
	if err != nil {
		return Attachment{}, false
	}
	spec, ok := obj.(syntax.Dict)
	if !ok {
		return Attachment{}, false
	}
	a := Attachment{Name: name, doc: d}
	if v, ok := spec.Get("UF").(syntax.String); ok {
		a.Name = decodeTextString(v)
	} else if v, ok := spec.Get("F").(syntax.String); ok {
		a.Name = decodeTextString(v)
	}
	if v, ok := spec.Get("Desc").(syntax.String); ok {
		a.Description = decodeTextString(v)
	}
	ef, err := d.file.Resolve(ctx, spec.Get("EF"))
	if err != nil {
		return Attachment{}, false
	}
	efd, ok := ef.(syntax.Dict)
	if !ok {
		return Attachment{}, false
	}
	streamObj := efd.Get("F")
	if efd.Has("UF") {
		streamObj = efd.Get("UF")
	}
	stm, err := d.file.Resolve(ctx, streamObj)
	if err != nil {
		return Attachment{}, false
	}
	a.stream, ok = stm.(*syntax.Stream)
	if !ok {
		return Attachment{}, false
	}
	if sub, ok := a.stream.Dict.NameValue("Subtype"); ok {
		a.MIME = sub
	}
	if params, err := d.file.Resolve(ctx, a.stream.Dict.Get("Params")); err == nil {
		if pd, ok := params.(syntax.Dict); ok {
			if n, ok := pd.IntValue("Size"); ok {
				a.Size = n
			}
			if v, ok := pd.Get("ModDate").(syntax.String); ok {
				a.ModDate = decodeTextString(v)
			}
		}
	}
	return a, true
}
