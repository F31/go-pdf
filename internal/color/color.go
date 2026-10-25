// Package color implements PDF color spaces and conversion to sRGB.
package color

import (
	"context"
	"math"

	"github.com/F31/go-pdf/internal/budget"
	"github.com/F31/go-pdf/internal/errs"
	"github.com/F31/go-pdf/internal/function"
	"github.com/F31/go-pdf/internal/syntax"
)

// Space converts color components in a PDF color space to sRGB.
type Space interface {
	Name() string
	Components() int
	Approx() bool
	RGBA(vals []float64) ([4]float64, bool, error)
}

// Manager optionally converts PDF color components to sRGB. profile is the
// decoded ICC stream for ICCBased spaces and nil otherwise.
type Manager interface {
	Convert(space string, profile []byte, n int, values []float64) ([4]float64, bool, error)
}

type approxReporter interface {
	Approximate(space string) bool
}

type managedSpace struct {
	base    Space
	manager Manager
	name    string
	profile []byte
	n       int
}

func (m managedSpace) ICCProfile() []byte { return m.profile }

func (m managedSpace) Name() string    { return m.name }
func (m managedSpace) Components() int { return m.n }
func (m managedSpace) Approx() bool {
	if a, ok := m.manager.(approxReporter); ok {
		return a.Approximate(m.name)
	}
	return true
}
func (m managedSpace) RGBA(vals []float64) ([4]float64, bool, error) {
	return m.manager.Convert(m.name, m.profile, m.n, vals)
}

// Load resolves a color space name or array.
func Load(ctx context.Context, file *syntax.File, obj syntax.Object, tracker *budget.Tracker, managers ...Manager) (Space, error) {
	var manager Manager
	if len(managers) > 0 {
		manager = managers[0]
	}
	obj, err := resolve(ctx, file, obj)
	if err != nil {
		return nil, err
	}
	switch v := obj.(type) {
	case syntax.Name:
		sp, err := byName(string(v), nil)
		if err == nil && manager != nil && string(v) == "DeviceCMYK" {
			return managedSpace{base: sp, manager: manager, name: string(v), n: sp.Components()}, nil
		}
		return sp, err
	case syntax.Array:
		if len(v) == 0 {
			return nil, clrErr("CLR-SPACE", "empty color space array")
		}
		name, ok := v[0].(syntax.Name)
		if !ok {
			return nil, clrErr("CLR-SPACE", "color space family is not a name")
		}
		return loadFamily(ctx, file, string(name), v, tracker, manager)
	default:
		return nil, clrErr("CLR-SPACE", "invalid color space object")
	}
}

func loadFamily(ctx context.Context, file *syntax.File, family string, arr syntax.Array, tracker *budget.Tracker, managers ...Manager) (Space, error) {
	switch family {
	case "CalGray", "CalRGB", "Lab":
		if len(arr) < 2 {
			return nil, clrErr("CLR-SPACE", "missing color space parameters")
		}
		dict, err := resolveDict(ctx, file, arr[1])
		if err != nil {
			return nil, err
		}
		switch family {
		case "CalGray":
			return loadCalGray(dict)
		case "CalRGB":
			return loadCalRGB(dict)
		default:
			return loadLab(dict)
		}
	case "ICCBased":
		return loadICC(ctx, file, arr, managers...)
	case "Indexed", "I":
		return loadIndexed(ctx, file, arr, tracker, managers...)
	case "Separation":
		return loadSeparation(ctx, file, arr, tracker, managers...)
	case "DeviceN":
		return loadDeviceN(ctx, file, arr, tracker, managers...)
	default:
		return byName(family, arr)
	}
}

func byName(name string, arr syntax.Array) (Space, error) {
	switch name {
	case "DeviceGray", "G":
		return device{name: "DeviceGray", n: 1}, nil
	case "DeviceRGB", "RGB":
		return device{name: "DeviceRGB", n: 3}, nil
	case "DeviceCMYK", "CMYK":
		return device{name: "DeviceCMYK", n: 4}, nil
	case "Pattern":
		return nil, errs.Wrap(errs.ErrUnsupported, "CLR-PATTERN", "pattern color space", -1, 0, -1)
	default:
		return nil, errs.Wrap(errs.ErrUnsupported, "CLR-SPACE", "unsupported color space", -1, 0, -1)
	}
}

type device struct {
	name string
	n    int
}

func (d device) Name() string    { return d.name }
func (d device) Components() int { return d.n }
func (d device) Approx() bool    { return d.name == "DeviceCMYK" }
func (d device) RGBA(vals []float64) ([4]float64, bool, error) {
	if len(vals) < d.n {
		return [4]float64{}, false, clrErr("CLR-COMPONENTS", "not enough color components")
	}
	switch d.name {
	case "DeviceGray":
		v := clamp(vals[0])
		return [4]float64{v, v, v, 1}, false, nil
	case "DeviceRGB":
		return [4]float64{clamp(vals[0]), clamp(vals[1]), clamp(vals[2]), 1}, false, nil
	default:
		c, m, y, k := clamp(vals[0]), clamp(vals[1]), clamp(vals[2]), clamp(vals[3])
		return [4]float64{(1 - c) * (1 - k), (1 - m) * (1 - k), (1 - y) * (1 - k), 1}, true, nil
	}
}

type calGray struct {
	white [3]float64
	black [3]float64
	gamma float64
}

func (c calGray) Name() string    { return "CalGray" }
func (c calGray) Components() int { return 1 }
func (c calGray) Approx() bool    { return !isD65(c.white) || c.black != [3]float64{} }
func (c calGray) RGBA(vals []float64) ([4]float64, bool, error) {
	if len(vals) < 1 {
		return [4]float64{}, false, clrErr("CLR-COMPONENTS", "not enough color components")
	}
	a := math.Pow(clamp(vals[0]), c.gamma)
	xyz := [3]float64{a * c.white[0], a * c.white[1], a * c.white[2]}
	return xyzRGBA(xyz, c.white), c.Approx(), nil
}

type calRGB struct {
	white  [3]float64
	black  [3]float64
	gamma  [3]float64
	matrix [9]float64
}

func (c calRGB) Name() string    { return "CalRGB" }
func (c calRGB) Components() int { return 3 }
func (c calRGB) Approx() bool    { return !isD65(c.white) || c.black != [3]float64{} }
func (c calRGB) RGBA(vals []float64) ([4]float64, bool, error) {
	if len(vals) < 3 {
		return [4]float64{}, false, clrErr("CLR-COMPONENTS", "not enough color components")
	}
	a := math.Pow(clamp(vals[0]), c.gamma[0])
	b := math.Pow(clamp(vals[1]), c.gamma[1])
	cc := math.Pow(clamp(vals[2]), c.gamma[2])
	xyz := [3]float64{
		c.matrix[0]*a + c.matrix[1]*b + c.matrix[2]*cc,
		c.matrix[3]*a + c.matrix[4]*b + c.matrix[5]*cc,
		c.matrix[6]*a + c.matrix[7]*b + c.matrix[8]*cc,
	}
	return xyzRGBA(xyz, c.white), c.Approx(), nil
}

type labSpace struct {
	white [3]float64
	black [3]float64
	rng   [4]float64
}

func (l labSpace) Name() string    { return "Lab" }
func (l labSpace) Components() int { return 3 }
func (l labSpace) Approx() bool    { return !isD50(l.white) }
func (l labSpace) RGBA(vals []float64) ([4]float64, bool, error) {
	if len(vals) < 3 {
		return [4]float64{}, false, clrErr("CLR-COMPONENTS", "not enough color components")
	}
	L := vals[0]
	if L < 0 {
		L = 0
	}
	if L > 100 {
		L = 100
	}
	a := clampRange(vals[1], l.rng[0], l.rng[1])
	b := clampRange(vals[2], l.rng[2], l.rng[3])
	fy := (L + 16) / 116
	fx := fy + a/500
	fz := fy - b/200
	xr, yr, zr := labInv(fx), labInv(fy), labInv(fz)
	xyz := [3]float64{xr * l.white[0], yr * l.white[1], zr * l.white[2]}
	return xyzRGBA(xyz, l.white), l.Approx(), nil
}

type icc struct {
	base    Space
	profile *iccProfile
	data    []byte
}

func (i icc) ICCProfile() []byte { return i.data }

func (i icc) Name() string    { return "ICCBased" }
func (i icc) Components() int { return i.base.Components() }
func (i icc) Approx() bool    { return i.profile == nil }
func (i icc) RGBA(vals []float64) ([4]float64, bool, error) {
	if i.profile != nil {
		return i.profile.rgba(vals)
	}
	rgba, _, err := i.base.RGBA(vals)
	return rgba, true, err
}

type indexed struct {
	base   Space
	hival  int
	lookup []byte
}

func (i indexed) Name() string    { return "Indexed" }
func (i indexed) Components() int { return 1 }
func (i indexed) Approx() bool    { return i.base.Approx() }
func (i indexed) RGBA(vals []float64) ([4]float64, bool, error) {
	if len(vals) < 1 {
		return [4]float64{}, false, clrErr("CLR-COMPONENTS", "not enough color components")
	}
	idx := int(math.Round(vals[0]))
	if idx < 0 || idx > i.hival {
		idx = 0
	}
	n := i.base.Components()
	if n <= 0 || (idx+1)*n > len(i.lookup) {
		return [4]float64{}, false, clrErr("CLR-INDEXED", "indexed lookup table is truncated")
	}
	comps := make([]float64, n)
	for j := 0; j < n; j++ {
		comps[j] = float64(i.lookup[idx*n+j]) / 255
	}
	rgba, approx, err := i.base.RGBA(comps)
	return rgba, approx, err
}

type tint struct {
	name   string
	alt    Space
	fn     function.Function
	inputs int
}

func (t tint) Name() string    { return t.name }
func (t tint) Components() int { return t.inputs }
func (t tint) Approx() bool    { return t.alt.Approx() }
func (t tint) RGBA(vals []float64) ([4]float64, bool, error) {
	if len(vals) < t.inputs {
		return [4]float64{}, false, clrErr("CLR-COMPONENTS", "not enough color components")
	}
	out, err := t.fn.Eval(vals[:t.inputs])
	if err != nil {
		return [4]float64{}, false, err
	}
	if len(out) != t.alt.Components() {
		return [4]float64{}, false, clrErr("CLR-TINT", "alternate space component mismatch")
	}
	rgba, approx, err := t.alt.RGBA(out)
	return rgba, approx || t.alt.Approx(), err
}

func loadCalGray(d syntax.Dict) (Space, error) {
	wp, err := numberArray(d.Get("WhitePoint"), 3, []float64{0.95047, 1, 1.08883})
	if err != nil {
		return nil, err
	}
	bp, err := numberArray(d.Get("BlackPoint"), 3, []float64{0, 0, 0})
	if err != nil {
		return nil, err
	}
	g, _ := number(d.Get("Gamma"))
	if g <= 0 {
		g = 1
	}
	return calGray{white: to3(wp), black: to3(bp), gamma: g}, nil
}

func loadCalRGB(d syntax.Dict) (Space, error) {
	wp, err := numberArray(d.Get("WhitePoint"), 3, []float64{0.95047, 1, 1.08883})
	if err != nil {
		return nil, err
	}
	bp, err := numberArray(d.Get("BlackPoint"), 3, []float64{0, 0, 0})
	if err != nil {
		return nil, err
	}
	gamma, err := numberArray(d.Get("Gamma"), 3, []float64{1, 1, 1})
	if err != nil {
		return nil, err
	}
	defMatrix := []float64{wp[0], 0, 0, 0, wp[1], 0, 0, 0, wp[2]}
	matrix, err := numberArray(d.Get("Matrix"), 9, defMatrix)
	if err != nil {
		return nil, err
	}
	for i := range gamma {
		if gamma[i] <= 0 {
			gamma[i] = 1
		}
	}
	return calRGB{white: to3(wp), black: to3(bp), gamma: to3(gamma), matrix: to9(matrix)}, nil
}

func loadLab(d syntax.Dict) (Space, error) {
	wp, err := numberArray(d.Get("WhitePoint"), 3, []float64{0.9642, 1, 0.8249})
	if err != nil {
		return nil, err
	}
	bp, err := numberArray(d.Get("BlackPoint"), 3, []float64{0, 0, 0})
	if err != nil {
		return nil, err
	}
	rng, err := numberArray(d.Get("Range"), 4, []float64{-100, 100, -100, 100})
	if err != nil {
		return nil, err
	}
	return labSpace{white: to3(wp), black: to3(bp), rng: [4]float64{rng[0], rng[1], rng[2], rng[3]}}, nil
}

func loadICC(ctx context.Context, file *syntax.File, arr syntax.Array, managers ...Manager) (Space, error) {
	if len(arr) < 2 {
		return nil, clrErr("CLR-ICC", "missing ICC stream")
	}
	stm, err := streamOf(ctx, file, arr[1])
	if err != nil {
		return nil, err
	}
	n, _ := stm.Dict.IntValue("N")
	base := iccBase(int(n))
	if file == nil {
		return icc{base: base}, nil
	}
	data, err := file.DecodeStream(ctx, stm)
	if err == nil && len(managers) > 0 && managers[0] != nil {
		return managedSpace{base: base, manager: managers[0], name: "ICCBased", profile: data, n: base.Components()}, nil
	}
	if err == nil {
		if profile, perr := parseICCProfileCached(data, int(n)); perr == nil {
			return icc{base: iccBase(profile.components()), profile: profile, data: data}, nil
		}
	}
	return icc{base: base}, nil
}

func iccBase(n int) Space {
	switch n {
	case 1:
		return device{name: "DeviceGray", n: 1}
	case 4:
		return device{name: "DeviceCMYK", n: 4}
	default:
		return device{name: "DeviceRGB", n: 3}
	}
}

func (p *iccProfile) components() int {
	if p.lut != nil {
		return p.lut.in
	}
	if p.isGray {
		return 1
	}
	return 3
}

func loadIndexed(ctx context.Context, file *syntax.File, arr syntax.Array, tracker *budget.Tracker, managers ...Manager) (Space, error) {
	if len(arr) < 4 {
		return nil, clrErr("CLR-INDEXED", "missing Indexed parameters")
	}
	base, err := Load(ctx, file, arr[1], tracker, managers...)
	if err != nil {
		return nil, err
	}
	hival, ok := number(arr[2])
	if !ok || hival < 0 {
		return nil, clrErr("CLR-INDEXED", "invalid hival")
	}
	lookup, err := bytesOf(ctx, file, arr[3])
	if err != nil {
		return nil, err
	}
	if len(lookup) < (int(hival)+1)*base.Components() {
		return nil, clrErr("CLR-INDEXED", "lookup table is truncated")
	}
	return indexed{base: base, hival: int(hival), lookup: lookup}, nil
}

func loadSeparation(ctx context.Context, file *syntax.File, arr syntax.Array, tracker *budget.Tracker, managers ...Manager) (Space, error) {
	if len(arr) < 4 {
		return nil, clrErr("CLR-SEPARATION", "missing Separation parameters")
	}
	alt, err := Load(ctx, file, arr[2], tracker, managers...)
	if err != nil {
		return nil, err
	}
	fn, err := function.Load(ctx, file, arr[3], tracker)
	if err != nil {
		return nil, err
	}
	return tint{name: "Separation", alt: alt, fn: fn, inputs: 1}, nil
}

func loadDeviceN(ctx context.Context, file *syntax.File, arr syntax.Array, tracker *budget.Tracker, managers ...Manager) (Space, error) {
	if len(arr) < 4 {
		return nil, clrErr("CLR-DEVICEN", "missing DeviceN parameters")
	}
	names, ok := arr[1].(syntax.Array)
	if !ok || len(names) == 0 {
		return nil, clrErr("CLR-DEVICEN", "invalid DeviceN colorants")
	}
	alt, err := Load(ctx, file, arr[2], tracker, managers...)
	if err != nil {
		return nil, err
	}
	fn, err := function.Load(ctx, file, arr[3], tracker)
	if err != nil {
		return nil, err
	}
	return tint{name: "DeviceN", alt: alt, fn: fn, inputs: len(names)}, nil
}

func resolve(ctx context.Context, file *syntax.File, obj syntax.Object) (syntax.Object, error) {
	if file == nil {
		return obj, nil
	}
	return file.Resolve(ctx, obj)
}

func resolveDict(ctx context.Context, file *syntax.File, obj syntax.Object) (syntax.Dict, error) {
	obj, err := resolve(ctx, file, obj)
	if err != nil {
		return nil, err
	}
	d, ok := obj.(syntax.Dict)
	if !ok {
		return nil, clrErr("CLR-SPACE", "color space dictionary expected")
	}
	return d, nil
}

func streamOf(ctx context.Context, file *syntax.File, obj syntax.Object) (*syntax.Stream, error) {
	obj, err := resolve(ctx, file, obj)
	if err != nil {
		return nil, err
	}
	stm, ok := obj.(*syntax.Stream)
	if !ok {
		return nil, clrErr("CLR-SPACE", "color space stream expected")
	}
	return stm, nil
}

func bytesOf(ctx context.Context, file *syntax.File, obj syntax.Object) ([]byte, error) {
	obj, err := resolve(ctx, file, obj)
	if err != nil {
		return nil, err
	}
	switch v := obj.(type) {
	case syntax.String:
		return []byte(v), nil
	case *syntax.Stream:
		if file == nil {
			return nil, clrErr("CLR-SPACE", "color space stream unavailable")
		}
		return file.DecodeStream(ctx, v)
	default:
		return nil, clrErr("CLR-SPACE", "color space bytes expected")
	}
}

func numberArray(o syntax.Object, want int, def []float64) ([]float64, error) {
	if o == nil {
		return def, nil
	}
	if _, null := o.(syntax.Null); null {
		return def, nil
	}
	arr, ok := o.(syntax.Array)
	if !ok || (want >= 0 && len(arr) != want) {
		return nil, clrErr("CLR-SPACE", "invalid color space array")
	}
	out := make([]float64, 0, len(arr))
	for _, item := range arr {
		n, ok := number(item)
		if !ok {
			return nil, clrErr("CLR-SPACE", "invalid color space number")
		}
		out = append(out, n)
	}
	return out, nil
}

func to3(in []float64) [3]float64 {
	var out [3]float64
	copy(out[:], in)
	return out
}

func to9(in []float64) [9]float64 {
	var out [9]float64
	copy(out[:], in)
	return out
}

func number(o syntax.Object) (float64, bool) {
	switch v := o.(type) {
	case syntax.Int:
		return float64(v), true
	case syntax.Real:
		return float64(v), true
	}
	return 0, false
}

func clampRange(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func xyzRGBA(xyz [3]float64, white [3]float64) [4]float64 {
	// Adapt the source white point to D65 with a von Kries scale. This is a
	// fixed approximation; ICC transforms are intentionally out of scope.
	if white[0] > 0 && white[1] > 0 && white[2] > 0 {
		xyz[0] *= 0.95047 / white[0]
		xyz[1] *= 1 / white[1]
		xyz[2] *= 1.08883 / white[2]
	}
	r := 3.2406*xyz[0] - 1.5372*xyz[1] - 0.4986*xyz[2]
	g := -0.9689*xyz[0] + 1.8758*xyz[1] + 0.0415*xyz[2]
	b := 0.0557*xyz[0] - 0.2040*xyz[1] + 1.0570*xyz[2]
	return [4]float64{srgb(r), srgb(g), srgb(b), 1}
}

func srgb(v float64) float64 {
	if v <= 0.0031308 {
		if v < 0 {
			return 0
		}
		return 12.92 * v
	}
	v = 1.055*math.Pow(v, 1/2.4) - 0.055
	if v > 1 {
		return 1
	}
	return v
}

func labInv(t float64) float64 {
	const e = 216.0 / 24389.0
	const k = 24389.0 / 27.0
	if t*t*t > e {
		return t * t * t
	}
	return (116*t - 16) / k
}

func isD65(w [3]float64) bool {
	return math.Abs(w[0]-0.95047) < 1e-4 && math.Abs(w[1]-1) < 1e-4 && math.Abs(w[2]-1.08883) < 1e-4
}

func isD50(w [3]float64) bool {
	return math.Abs(w[0]-0.9642) < 1e-4 && math.Abs(w[1]-1) < 1e-4 && math.Abs(w[2]-0.8249) < 1e-4
}

func clrErr(code, msg string) error {
	return errs.Wrap(errs.ErrCorrupt, code, msg, -1, 0, -1)
}
