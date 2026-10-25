package color

import "errors"

// Transform converts source ICC components to sRGB for Preview. Target-device
// and print-PCS transforms are intentionally out of scope.
type Transform struct {
	p *iccProfile
	n int
}

// NewTransform parses an ICC profile for group/color-pipeline use.
func NewTransform(profile []byte, n int) (*Transform, error) {
	p, err := parseICCProfileCached(profile, n)
	if err != nil {
		return nil, err
	}
	return &Transform{p: p, n: p.components()}, nil
}

// Components returns the device component count.
func (t *Transform) Components() int {
	if t == nil {
		return 0
	}
	return t.n
}

// ToSRGB converts device components to sRGB.
func (t *Transform) ToSRGB(vals []float64) ([4]float64, bool, error) {
	if t == nil || t.p == nil {
		return [4]float64{}, false, errors.New("color: nil transform")
	}
	return t.p.rgba(vals)
}
