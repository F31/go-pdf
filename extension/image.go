package extension

import (
	"context"
	"io"
)

// ImageEncoding is a PDF image filter name, for example "JBIG2Decode" or
// "JPXDecode".
type ImageEncoding string

const (
	ImageJBIG2     ImageEncoding = "JBIG2Decode"
	ImageJPX       ImageEncoding = "JPXDecode"
	ImageCCITTG32D ImageEncoding = "CCITTFaxDecode"
)

// ImageConfig carries the decoded image geometry and colour metadata. Codec
// parameters that are not relevant to a provider may be zero.
type ImageConfig struct {
	Encoding         ImageEncoding
	Width            int
	Height           int
	Components       int
	BitsPerComponent int
	ColorSpace       string
	Decode           []float64
	ICCProfile       []byte
	ImageMask        bool
	Interpolate      bool
}

// ImagePlane is an interleaved, row-major, top-down sample plane. Pix uses one
// byte per sample for 1/2/4/8 bits and big-endian uint16 pairs for 16 bits.
// Stride is measured in bytes. The plane owns Pix until it is released by its
// decoder.
type ImagePlane struct {
	Pix              []byte
	Stride           int
	Width            int
	Height           int
	Components       int
	BitsPerComponent int
	ColorSpace       string
	Alpha            bool
	Premultiplied    bool
}

// ImageDecoder is one mutable decode session. It is not safe for concurrent use
// unless its provider explicitly documents otherwise.
type ImageDecoder interface {
	Decode(ctx context.Context, src io.ReaderAt, size int64, cfg ImageConfig) (*ImagePlane, error)
	Close() error
}

// ImageDecoderFactory creates a decoder per task. The factory is immutable and
// may be shared by concurrent documents.
type ImageDecoderFactory interface {
	Provider
	Encodings() []ImageEncoding
	NewDecoder(ctx context.Context, cfg ImageConfig) (ImageDecoder, error)
}
