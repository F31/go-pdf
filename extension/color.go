package extension

import "context"

// ColorSpec identifies one source or destination colour space. Data is the raw
// ICC profile when Name is "ICCBased"; it is nil for device spaces and sRGB.
type ColorSpec struct {
	Name       string
	Components int
	Data       []byte
}

// ColorTransform converts interleaved, normalized [0,1] float samples. The
// caller provides exactly pixels*SourceComponents values in src and
// pixels*DestinationComponents values in dst.
type ColorTransform interface {
	SourceComponents() int
	DestinationComponents() int
	Transform(ctx context.Context, dst, src []float32, pixels int) error
	Close() error
}

// ColorEngine creates reusable colour transforms. Implementations should cache
// transforms by profile digest and parameters under a bounded cache budget.
type ColorEngine interface {
	Provider
	NewTransform(ctx context.Context, src, dst ColorSpec) (ColorTransform, error)
}
