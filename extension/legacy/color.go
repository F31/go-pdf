package legacy

import (
	"context"
	"errors"

	"github.com/F31/go-pdf"
	"github.com/F31/go-pdf/extension"
)

// NewColorEngine adapts a legacy pdf.ColorManager to a colour engine that only
// produces sRGB. Other destination spaces are rejected instead of approximated.
func NewColorEngine(info extension.ProviderInfo, m pdf.ColorManager) extension.ColorEngine {
	if m == nil {
		return nil
	}
	return colorEngine{info: info, legacy: m}
}

type colorEngine struct {
	info   extension.ProviderInfo
	legacy pdf.ColorManager
}

func (e colorEngine) Info() extension.ProviderInfo  { return e.info }
func (e colorEngine) Features() []extension.Feature { return nil }

func (e colorEngine) NewTransform(_ context.Context, src, dst extension.ColorSpec) (extension.ColorTransform, error) {
	if src.Components <= 0 {
		return nil, errors.New("legacy: source colour has no components")
	}
	if !isSRGB(dst) {
		return nil, errors.New("legacy: only sRGB destination is supported")
	}
	return &colorTransform{manager: e.legacy, src: src}, nil
}

type colorTransform struct {
	manager pdf.ColorManager
	src     extension.ColorSpec
}

func (t *colorTransform) SourceComponents() int      { return t.src.Components }
func (t *colorTransform) DestinationComponents() int { return 3 }

func (t *colorTransform) Transform(ctx context.Context, dst, src []float32, pixels int) error {
	if pixels < 0 || len(src) != pixels*t.src.Components || len(dst) != pixels*3 {
		return errors.New("legacy: invalid colour buffer length")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	values := make([]float64, t.src.Components)
	for i := 0; i < pixels; i++ {
		in := src[i*t.src.Components : (i+1)*t.src.Components]
		for j, v := range in {
			values[j] = float64(v)
		}
		rgba, _, err := t.manager.Convert(pdf.ColorSpaceInfo{
			Name:       t.src.Name,
			Components: t.src.Components,
			ICCProfile: t.src.Data,
		}, values)
		if err != nil {
			return err
		}
		out := dst[i*3 : (i+1)*3]
		out[0], out[1], out[2] = float32(rgba[0]), float32(rgba[1]), float32(rgba[2])
	}
	return nil
}

func (*colorTransform) Close() error { return nil }

func isSRGB(spec extension.ColorSpec) bool {
	switch spec.Name {
	case "sRGB", "DeviceRGB", "RGB":
		return true
	default:
		return false
	}
}
