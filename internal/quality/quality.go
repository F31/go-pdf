// Package quality provides image comparison metrics used by render quality
// gates. It is intentionally independent of the renderer.
package quality

import (
	"errors"
	"image"
	"image/color"
	"math"
	"sort"
)

// Report summarizes per-pixel differences between two equally sized images.
type Report struct {
	Width, Height int
	PSNR          float64
	SSIM          float64
	MeanDeltaE    float64
	P95DeltaE     float64
}

// Compare computes PSNR, global SSIM and CIEDE2000 statistics over RGBA pixels.
func Compare(a, b image.Image) (Report, error) {
	if a == nil || b == nil {
		return Report{}, errors.New("quality: nil image")
	}
	ab, bb := a.Bounds(), b.Bounds()
	if ab.Dx() != bb.Dx() || ab.Dy() != bb.Dy() {
		return Report{}, errors.New("quality: image dimensions differ")
	}
	w, h := ab.Dx(), ab.Dy()
	if w == 0 || h == 0 {
		return Report{}, errors.New("quality: empty image")
	}
	var mse, sumA, sumB, sumAA, sumBB, sumAB float64
	var lc labCache
	deltas := make([]float64, 0, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			ar, ag, abv, aa := rgba(a.At(ab.Min.X+x, ab.Min.Y+y))
			br, bg, bbv, ba := rgba(b.At(bb.Min.X+x, bb.Min.Y+y))
			dr, dg, db, da := ar-br, ag-bg, abv-bbv, aa-ba
			mse += dr*dr + dg*dg + db*db + da*da
			la := luma(ar, ag, abv)
			lb := luma(br, bg, bbv)
			sumA += la
			sumB += lb
			sumAA += la * la
			sumBB += lb * lb
			sumAB += la * lb
			if ar == br && ag == bg && abv == bbv && aa == ba {
				deltas = append(deltas, 0)
				continue
			}
			deltas = append(deltas, deltaE2000Cached(&lc, ar, ag, abv, br, bg, bbv))
		}
	}
	n := float64(w * h)
	mse /= n * 4
	report := Report{Width: w, Height: h}
	if mse == 0 {
		report.PSNR = math.Inf(1)
	} else {
		report.PSNR = 10 * math.Log10(255*255/mse)
	}
	meanA, meanB := sumA/n, sumB/n
	varA := math.Max(0, sumAA/n-meanA*meanA)
	varB := math.Max(0, sumBB/n-meanB*meanB)
	cov := sumAB/n - meanA*meanB
	const c1 = (0.01 * 255) * (0.01 * 255)
	const c2 = (0.03 * 255) * (0.03 * 255)
	report.SSIM = ((2*meanA*meanB + c1) * (2*cov + c2)) / ((meanA*meanA + meanB*meanB + c1) * (varA + varB + c2))
	sort.Float64s(deltas)
	for _, d := range deltas {
		report.MeanDeltaE += d
	}
	report.MeanDeltaE /= n
	idx := int(math.Ceil(0.95*n)) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(deltas) {
		idx = len(deltas) - 1
	}
	report.P95DeltaE = deltas[idx]
	return report, nil
}

func rgba(c color.Color) (float64, float64, float64, float64) {
	n := color.NRGBA64Model.Convert(c).(color.NRGBA64)
	return float64(n.R) / 257, float64(n.G) / 257, float64(n.B) / 257, float64(n.A) / 257
}

func luma(r, g, b float64) float64 { return 0.2126*r + 0.7152*g + 0.0722*b }

func deltaE2000Cached(lc *labCache, r1, g1, b1, r2, g2, b2 float64) float64 {
	l1, a1, bb1 := lc.lab(r1, g1, b1)
	l2, a2, bb2 := lc.lab(r2, g2, b2)
	c1 := math.Hypot(a1, bb1)
	c2 := math.Hypot(a2, bb2)
	cbar := (c1 + c2) / 2
	cbar7 := pow7(cbar)
	g := 0.5 * (1 - math.Sqrt(cbar7/(cbar7+pow25_7)))
	a1p, a2p := (1+g)*a1, (1+g)*a2
	c1p, c2p := math.Hypot(a1p, bb1), math.Hypot(a2p, bb2)
	h1p, h2p := hue(a1p, bb1), hue(a2p, bb2)
	dlp := l2 - l1
	dcp := c2p - c1p
	dhp := h2p - h1p
	if c1p*c2p == 0 {
		dhp = 0
	} else if dhp > math.Pi {
		dhp -= 2 * math.Pi
	} else if dhp < -math.Pi {
		dhp += 2 * math.Pi
	}
	dHp := 2 * math.Sqrt(c1p*c2p) * math.Sin(dhp/2)
	lbarp := (l1 + l2) / 2
	cbarp := (c1p + c2p) / 2
	var hbarp float64
	if c1p*c2p == 0 {
		hbarp = h1p + h2p
	} else if math.Abs(h1p-h2p) <= math.Pi {
		hbarp = (h1p + h2p) / 2
	} else if h1p+h2p < 2*math.Pi {
		hbarp = (h1p + h2p + 2*math.Pi) / 2
	} else {
		hbarp = (h1p + h2p - 2*math.Pi) / 2
	}
	t := 1 - 0.17*math.Cos(hbarp-math.Pi/6) + 0.24*math.Cos(2*hbarp) + 0.32*math.Cos(3*hbarp+math.Pi/30) - 0.20*math.Cos(4*hbarp-63*math.Pi/180)
	dtv := (hbarp*180/math.Pi - 275) / 25
	dt := 30 * math.Exp(-dtv*dtv)
	cbarp7 := pow7(cbarp)
	rc := 2 * math.Sqrt(cbarp7/(cbarp7+pow25_7))
	dlv := lbarp - 50
	sl := 1 + 0.015*dlv*dlv/math.Sqrt(20+dlv*dlv)
	sc := 1 + 0.045*cbarp
	sh := 1 + 0.015*cbarp*t
	rt := -math.Sin(2*dt*math.Pi/180) * rc
	dl := dlp / sl
	dc := dcp / sc
	dh := dHp / sh
	return math.Sqrt(dl*dl + dc*dc + dh*dh + rt*dc*dh)
}

const pow25_7 = 6103515625.0

func pow7(x float64) float64 {
	x2 := x * x
	x3 := x2 * x
	return x3 * x3 * x
}

type labEntry struct {
	key     uint32
	l, a, b float64
	set     bool
}

type labCache struct {
	entries [4096]labEntry
}

func (c *labCache) lab(r, g, b float64) (float64, float64, float64) {
	if !isByte(r) || !isByte(g) || !isByte(b) {
		return lab(r, g, b)
	}
	key := uint32(r)<<16 | uint32(g)<<8 | uint32(b)
	idx := (key * 2654435761) >> 20 & 4095
	e := &c.entries[idx]
	if e.set && e.key == key {
		return e.l, e.a, e.b
	}
	l, a, bb := lab(r, g, b)
	*e = labEntry{key: key, l: l, a: a, b: bb, set: true}
	return l, a, bb
}

func isByte(v float64) bool {
	return v >= 0 && v <= 255 && v == math.Trunc(v)
}

var srgbLinearLUT = func() [256]float64 {
	var lut [256]float64
	for i := range lut {
		lut[i] = srgbLinear(float64(i) / 255)
	}
	return lut
}()

func srgbLinear255(v float64) float64 {
	if isByte(v) {
		return srgbLinearLUT[int(v)]
	}
	return srgbLinear(v / 255)
}

func lab(r, g, b float64) (float64, float64, float64) {
	r = srgbLinear255(r)
	g = srgbLinear255(g)
	b = srgbLinear255(b)
	x := 0.4124564*r + 0.3575761*g + 0.1804375*b
	y := 0.2126729*r + 0.7151522*g + 0.0721750*b
	z := 0.0193339*r + 0.1191920*g + 0.9503041*b
	fx := labF(x / 0.95047)
	fy := labF(y / 1)
	fz := labF(z / 1.08883)
	return 116*fy - 16, 500 * (fx - fy), 200 * (fy - fz)
}

func srgbLinear(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

func labF(t float64) float64 {
	if t > 216.0/24389.0 {
		return math.Cbrt(t)
	}
	return (24389.0/27*t + 16) / 116
}

func hue(a, b float64) float64 {
	h := math.Atan2(b, a)
	if h < 0 {
		h += 2 * math.Pi
	}
	return h
}
