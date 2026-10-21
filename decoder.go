package pdf

import (
	"context"
	"image"

	"github.com/F31/go-pdf/displaylist"
	pdfimage "github.com/F31/go-pdf/internal/image"
)

// ExternalDecoder decodes image encodings that are not implemented by the
// core package, such as JBIG2 and JPX. Implementations must be safe for
// concurrent use.
type ExternalDecoder interface {
	CanDecode(displaylist.ImageEncoding) bool
	Decode(ctx context.Context, raw []byte, info displaylist.ImageInfo) (image.Image, error)
}

// RegisterExternalDecoder installs a decoder for passthrough image encodings.
// Decoders are consulted in registration order. Register during program
// initialization, before concurrent page decoding starts.
func RegisterExternalDecoder(d ExternalDecoder) {
	pdfimage.RegisterExternalDecoder(d)
}
