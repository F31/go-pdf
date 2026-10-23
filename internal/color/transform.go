package color

import (
	"errors"
	"math"
)

// Transform is a reusable ICC profile transform between device components and
// sRGB. It is intentionally limited to profiles the ICC parser understands.
type Transform struct {
	p *iccProfile
	n int
}

// NewTransform parses an ICC profile for group/color-pipeline use.
func NewTransform(profile []byte, n int) (*Transform, error) {
	p, err := parseICCProfile(profile, n)
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

// ToLabPCS converts device components to normalized Lab PCS when the profile
// exposes an A2B Lab LUT. This allows direct source-to-target ICC chaining.
func (t *Transform) ToLabPCS(vals []float64) ([]float64, error) {
	if t == nil || t.p == nil {
		return nil, errors.New("color: nil transform")
	}
	p := t.p
	if p.lut != nil {
		out, err := p.lut.evalRaw(vals)
		if err != nil {
			return nil, err
		}
		if len(out) < 3 {
			return nil, errors.New("color: ICC A2B returned too few components")
		}
		if p.lut.pcs == "Lab " {
			return out[:3], nil
		}
		return xyzToLabPCS([3]float64{out[0], out[1], out[2]}, p.lut.white), nil
	}
	if p.isGray {
		if len(vals) < 1 {
			return nil, errors.New("color: not enough gray components")
		}
		l := p.grayTRC.eval(clamp(vals[0]))
		return xyzToLabPCS([3]float64{p.white[0] * l, p.white[1] * l, p.white[2] * l}, p.white), nil
	}
	if len(vals) < 3 {
		return nil, errors.New("color: not enough RGB components")
	}
	r := p.trc[0].eval(clamp(vals[0]))
	g := p.trc[1].eval(clamp(vals[1]))
	b := p.trc[2].eval(clamp(vals[2]))
	xyz := [3]float64{
		p.matrix[0]*r + p.matrix[1]*g + p.matrix[2]*b,
		p.matrix[3]*r + p.matrix[4]*g + p.matrix[5]*b,
		p.matrix[6]*r + p.matrix[7]*g + p.matrix[8]*b,
	}
	return xyzToLabPCS(xyz, p.white), nil
}

// FromLabPCS converts normalized Lab PCS to device components using B2A.
func (t *Transform) FromLabPCS(pcs []float64) ([]float64, error) {
	if t == nil || t.p == nil {
		return nil, errors.New("color: nil transform")
	}
	if t.p.btoa == nil || t.p.pcs != "Lab " {
		return nil, errors.New("color: ICC profile has no usable Lab B2A LUT")
	}
	if len(pcs) < 3 {
		return nil, errors.New("color: not enough Lab PCS components")
	}
	return t.p.btoa.eval(pcs)
}

// FromSRGB converts sRGB to device components using B2A when available.
func (t *Transform) FromSRGB(rgba [4]float64) ([]float64, error) {
	if t == nil || t.p == nil {
		return nil, errors.New("color: nil transform")
	}
	pcs := srgbToPCS(rgba, t.p.white)
	return t.FromLabPCS(pcs)
}

func xyzToLabPCS(xyz [3]float64, white [3]float64) []float64 {
	// ICC PCS XYZ is D50-relative. Adapt the media white to D50 first, then
	// convert to normalized D50 Lab, matching B2A Lab LUT input.
	x, y, z := xyz[0], xyz[1], xyz[2]
	if white[0] > 0 && white[1] > 0 && white[2] > 0 {
		x *= 0.9642 / white[0]
		y *= 1 / white[1]
		z *= 0.8249 / white[2]
	}
	fx := labF(x / 0.9642)
	fy := labF(y / 1.0)
	fz := labF(z / 0.8249)
	l := 116*fy - 16
	a := 500 * (fx - fy)
	b := 200 * (fy - fz)
	return []float64{l / 100, (a + 128) / 255, (b + 128) / 255}
}

func srgbToPCS(rgba [4]float64, white [3]float64) []float64 {
	r := srgbLinear(rgba[0])
	g := srgbLinear(rgba[1])
	b := srgbLinear(rgba[2])
	// sRGB D65 to XYZ D65.
	x := 0.4124564*r + 0.3575761*g + 0.1804375*b
	y := 0.2126729*r + 0.7151522*g + 0.0721750*b
	z := 0.0193339*r + 0.1191920*g + 0.9503041*b
	// Adapt D65 to the profile PCS white by inverting xyzRGBA's scale.
	x *= white[0] / 0.95047
	y *= white[1] / 1.0
	z *= white[2] / 1.08883
	// D50 Lab.
	fx := labF(x / 0.9642)
	fy := labF(y / 1.0)
	fz := labF(z / 0.8249)
	L := 116*fy - 16
	a := 500 * (fx - fy)
	bb := 200 * (fy - fz)
	return []float64{L / 100, (a + 128) / 255, (bb + 128) / 255}
}

func srgbLinear(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

func labF(t float64) float64 {
	if t > 0.008856 {
		return math.Cbrt(t)
	}
	return (903.3*t + 16) / 116
}
