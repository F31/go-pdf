package image

import (
	"context"
	"image"
	"sync"

	"github.com/F31/go-pdf/displaylist"
)

// ExternalDecoder decodes image encodings that are not implemented in the
// core package, such as JBIG2 and JPX. Implementations are supplied by the
// caller and must be safe for concurrent use.
type ExternalDecoder interface {
	CanDecode(displaylist.ImageEncoding) bool
	Decode(ctx context.Context, raw []byte, info displaylist.ImageInfo) (image.Image, error)
}

var (
	externalMu sync.RWMutex
	external   []ExternalDecoder
)

// RegisterExternalDecoder adds a decoder. Nil values are ignored. Decoders
// are consulted in registration order.
func RegisterExternalDecoder(d ExternalDecoder) {
	if d == nil {
		return
	}
	externalMu.Lock()
	external = append(external, d)
	externalMu.Unlock()
}

func externalDecoder(enc displaylist.ImageEncoding) ExternalDecoder {
	externalMu.RLock()
	defer externalMu.RUnlock()
	for _, d := range external {
		if d.CanDecode(enc) {
			return d
		}
	}
	return nil
}
