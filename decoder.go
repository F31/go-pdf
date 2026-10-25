package pdf

import (
	"context"
	"errors"
	"image"

	"github.com/F31/go-pdf/displaylist"
)

// ExternalDecoder decodes one image task. Implementations must be safe for
// concurrent use if the factory returns the same instance to multiple tasks.
type ExternalDecoder interface {
	Decode(ctx context.Context, raw []byte, info displaylist.ImageInfo) (image.Image, error)
}

// DecoderFactory creates decoder sessions for one or more encodings. Factories
// should create a new session by default; stateless decoders may return a
// shared instance explicitly.
type DecoderFactory interface {
	Encodings() []displaylist.ImageEncoding
	NewDecoder(ctx context.Context, info displaylist.ImageInfo) (ExternalDecoder, error)
}

// DecoderRegistry is an immutable encoder-to-factory catalogue. It can be
// shared by multiple Documents. Use WithOverride to derive a new registry
// instead of mutating a shared one.
type DecoderRegistry struct {
	factories map[displaylist.ImageEncoding]DecoderFactory
}

// NewDecoderRegistry builds an immutable registry. Duplicate encodings are
// rejected.
func NewDecoderRegistry(factories ...DecoderFactory) (*DecoderRegistry, error) {
	r := &DecoderRegistry{factories: map[displaylist.ImageEncoding]DecoderFactory{}}
	for _, f := range factories {
		if f == nil {
			return nil, errors.New("pdf: nil decoder factory")
		}
		encs := f.Encodings()
		if len(encs) == 0 {
			return nil, errors.New("pdf: decoder factory has no encodings")
		}
		for _, enc := range encs {
			if _, exists := r.factories[enc]; exists {
				return nil, errors.New("pdf: duplicate decoder encoding")
			}
			r.factories[enc] = f
		}
	}
	return r, nil
}

// WithOverride derives a new registry with enc replaced by factory.
func (r *DecoderRegistry) WithOverride(enc displaylist.ImageEncoding, factory DecoderFactory) (*DecoderRegistry, error) {
	if r == nil {
		return nil, errors.New("pdf: nil decoder registry")
	}
	if factory == nil {
		return nil, errors.New("pdf: nil decoder factory")
	}
	next := &DecoderRegistry{factories: make(map[displaylist.ImageEncoding]DecoderFactory, len(r.factories)+1)}
	for k, v := range r.factories {
		next.factories[k] = v
	}
	next.factories[enc] = factory
	return next, nil
}

func (r *DecoderRegistry) hasEncoding(enc displaylist.ImageEncoding) bool {
	if r == nil {
		return false
	}
	_, ok := r.factories[enc]
	return ok
}

func (r *DecoderRegistry) decoder(ctx context.Context, enc displaylist.ImageEncoding, info displaylist.ImageInfo) (ExternalDecoder, error) {
	if r == nil {
		return nil, nil
	}
	f := r.factories[enc]
	if f == nil {
		return nil, nil
	}
	return f.NewDecoder(ctx, info)
}

// WithDecoderRegistry uses an immutable decoder registry for the document's
// configuration snapshot.
func WithDecoderRegistry(r *DecoderRegistry) Option {
	return func(o *options) error {
		o.decoders = r
		return nil
	}
}
