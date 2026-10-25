package image

import (
	"context"
	"image"

	"github.com/F31/go-pdf/displaylist"
)

// ExternalDecoder decodes image encodings that are not implemented in the
// core package, such as JBIG2 and JPX. Implementations are supplied by the
// caller and must be safe for concurrent use.
type ExternalDecoder interface {
	Decode(ctx context.Context, raw []byte, info displaylist.ImageInfo) (image.Image, error)
}

// DecoderLookup creates a decoder for one image task. A document-scoped
// registry supplies this callback; nil means no external decoder is available.
type DecoderLookup func(context.Context, displaylist.ImageEncoding, displaylist.ImageInfo) (ExternalDecoder, error)

// ExternalSampler is an optional extension of ExternalDecoder for providers
// that can expose normalized source samples without first converting to sRGB.
type ExternalSampler interface {
	DecodeSamples(ctx context.Context, raw []byte, info displaylist.ImageInfo) (*displaylist.ImageSamples, error)
}
