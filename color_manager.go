package pdf

import pdfcolor "github.com/F31/go-pdf/internal/color"

// ColorSpaceInfo identifies the color space passed to a ColorManager.
type ColorSpaceInfo struct {
	Name       string
	Components int
	ICCProfile []byte
}

// ColorManager optionally converts PDF color components to sRGB. It is called
// for DeviceCMYK and ICCBased spaces. Returning an error aborts the operation;
// callers must not silently fall back.
type ColorManager interface {
	Convert(info ColorSpaceInfo, values []float64) ([4]float64, bool, error)
}

type colorManagerAdapter struct {
	manager ColorManager
}

func (a colorManagerAdapter) Convert(space string, profile []byte, n int, values []float64) ([4]float64, bool, error) {
	return a.manager.Convert(ColorSpaceInfo{Name: space, Components: n, ICCProfile: profile}, values)
}

func wrapColorManager(m ColorManager) pdfcolor.Manager {
	if m == nil {
		return nil
	}
	return colorManagerAdapter{manager: m}
}
