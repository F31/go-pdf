// Package function implements PDF Function objects (types 0-4).
package function

import (
	"context"
	"errors"
	"math"

	"github.com/F31/go-pdf/internal/budget"
	"github.com/F31/go-pdf/internal/errs"
	"github.com/F31/go-pdf/internal/syntax"
)

// Function maps m input values to n output values.
type Function interface {
	Inputs() int
	Outputs() int
	Eval(in []float64) ([]float64, error)
}

// Load resolves and parses a PDF Function object.
func Load(ctx context.Context, file *syntax.File, obj syntax.Object, tracker *budget.Tracker) (Function, error) {
	obj, err := resolve(ctx, file, obj)
	if err != nil {
		return nil, err
	}
	var dict syntax.Dict
	var stm *syntax.Stream
	switch v := obj.(type) {
	case syntax.Dict:
		dict = v
	case *syntax.Stream:
		dict, stm = v.Dict, v
	default:
		return nil, errs.Wrap(errs.ErrCorrupt, "FUNC-TYPE", "function is not a dictionary", -1, 0, -1)
	}
	typ, ok := intValue(dict, "FunctionType")
	if !ok {
		return nil, errs.Wrap(errs.ErrCorrupt, "FUNC-TYPE", "function type missing", -1, 0, -1)
	}
	switch typ {
	case 0:
		return loadSampled(ctx, file, dict, stm, tracker)
	case 2:
		return loadExponential(dict)
	case 3:
		return loadStitching(ctx, file, dict, tracker)
	case 4:
		return loadPostScript(ctx, file, dict, stm, tracker)
	default:
		return nil, errs.Wrap(errs.ErrUnsupported, "FUNC-TYPE", "unsupported function type", -1, 0, -1)
	}
}

func resolve(ctx context.Context, file *syntax.File, obj syntax.Object) (syntax.Object, error) {
	if file == nil {
		return obj, nil
	}
	return file.Resolve(ctx, obj)
}

func loadExponential(dict syntax.Dict) (Function, error) {
	domain, err := floatArray(dict.Get("Domain"), 2, []float64{0, 1})
	if err != nil {
		return nil, err
	}
	c0, err := floatArray(dict.Get("C0"), -1, []float64{0})
	if err != nil {
		return nil, err
	}
	c1, err := floatArray(dict.Get("C1"), len(c0), []float64{1})
	if err != nil {
		return nil, err
	}
	if len(c0) != len(c1) {
		return nil, errs.Wrap(errs.ErrCorrupt, "FUNC-C0C1", "C0 and C1 lengths differ", -1, 0, -1)
	}
	rng, err := floatArray(dict.Get("Range"), -1, nil)
	if err != nil {
		return nil, err
	}
	n := 1.0
	if v, ok := number(dict.Get("N")); ok {
		n = v
	}
	if n <= 0 {
		return nil, errs.Wrap(errs.ErrCorrupt, "FUNC-N", "invalid exponent", -1, 0, -1)
	}
	return &exponential{domain: domain, c0: c0, c1: c1, n: n, rng: rng}, nil
}

type exponential struct {
	domain []float64
	c0, c1 []float64
	n      float64
	rng    []float64
}

func (f *exponential) Inputs() int  { return 1 }
func (f *exponential) Outputs() int { return len(f.c0) }
func (f *exponential) Eval(in []float64) ([]float64, error) {
	if len(in) < 1 {
		return nil, errs.Wrap(errs.ErrCorrupt, "FUNC-INPUT", "missing function input", -1, 0, -1)
	}
	t := norm(in[0], f.domain[0], f.domain[1])
	t = math.Pow(t, f.n)
	out := make([]float64, len(f.c0))
	for i := range out {
		out[i] = f.c0[i] + t*(f.c1[i]-f.c0[i])
	}
	clip(out, f.rng)
	return out, nil
}

func loadStitching(ctx context.Context, file *syntax.File, dict syntax.Dict, tracker *budget.Tracker) (Function, error) {
	domain, err := floatArray(dict.Get("Domain"), 2, []float64{0, 1})
	if err != nil {
		return nil, err
	}
	bounds, err := floatArray(dict.Get("Bounds"), -1, nil)
	if err != nil {
		return nil, err
	}
	enc, err := floatArray(dict.Get("Encode"), -1, nil)
	if err != nil {
		return nil, err
	}
	rng, err := floatArray(dict.Get("Range"), -1, nil)
	if err != nil {
		return nil, err
	}
	obj, err := resolve(ctx, file, dict.Get("Functions"))
	if err != nil {
		return nil, err
	}
	arr, ok := obj.(syntax.Array)
	if !ok || len(arr) == 0 {
		return nil, errs.Wrap(errs.ErrCorrupt, "FUNC-STITCH", "stitching functions missing", -1, 0, -1)
	}
	funcs := make([]Function, len(arr))
	for i, o := range arr {
		sub, err := Load(ctx, file, o, tracker)
		if err != nil {
			return nil, err
		}
		if sub.Inputs() != 1 {
			return nil, errs.Wrap(errs.ErrCorrupt, "FUNC-STITCH", "subfunction must have one input", -1, 0, -1)
		}
		funcs[i] = sub
	}
	if len(bounds) != len(funcs)-1 {
		return nil, errs.Wrap(errs.ErrCorrupt, "FUNC-STITCH", "bounds length mismatch", -1, 0, -1)
	}
	if len(enc) == 0 {
		enc = make([]float64, 2*len(funcs))
		for i := range funcs {
			enc[2*i] = domain[0]
			enc[2*i+1] = domain[1]
		}
	}
	if len(enc) != 2*len(funcs) {
		return nil, errs.Wrap(errs.ErrCorrupt, "FUNC-STITCH", "encode length mismatch", -1, 0, -1)
	}
	return &stitching{domain: domain, bounds: bounds, encode: enc, rng: rng, funcs: funcs}, nil
}

type stitching struct {
	domain []float64
	bounds []float64
	encode []float64
	rng    []float64
	funcs  []Function
}

func (f *stitching) Inputs() int  { return 1 }
func (f *stitching) Outputs() int { return f.funcs[0].Outputs() }
func (f *stitching) Eval(in []float64) ([]float64, error) {
	if len(in) < 1 {
		return nil, errs.Wrap(errs.ErrCorrupt, "FUNC-INPUT", "missing function input", -1, 0, -1)
	}
	x := in[0]
	if x < f.domain[0] {
		x = f.domain[0]
	}
	if x > f.domain[1] {
		x = f.domain[1]
	}
	i := 0
	for i < len(f.bounds) && x >= f.bounds[i] {
		i++
	}
	lo := f.domain[0]
	if i > 0 {
		lo = f.bounds[i-1]
	}
	hi := f.domain[1]
	if i < len(f.bounds) {
		hi = f.bounds[i]
	}
	t := 0.0
	if hi > lo {
		t = (x - lo) / (hi - lo)
	}
	enc := f.encode[2*i] + t*(f.encode[2*i+1]-f.encode[2*i])
	out, err := f.funcs[i].Eval([]float64{enc})
	if err != nil {
		return nil, err
	}
	clip(out, f.rng)
	return out, nil
}

func loadSampled(ctx context.Context, file *syntax.File, dict syntax.Dict, stm *syntax.Stream, tracker *budget.Tracker) (Function, error) {
	if stm == nil || file == nil {
		return nil, errs.Wrap(errs.ErrCorrupt, "FUNC-SAMPLE", "sampled function stream missing", -1, 0, -1)
	}
	domain, err := floatArray(dict.Get("Domain"), -1, nil)
	if err != nil {
		return nil, err
	}
	if len(domain) == 0 || len(domain)%2 != 0 {
		return nil, errs.Wrap(errs.ErrCorrupt, "FUNC-DOMAIN", "invalid function domain", -1, 0, -1)
	}
	inputs := len(domain) / 2
	if inputs > 8 {
		return nil, errs.Wrap(errs.ErrUnsupported, "FUNC-DOMAIN", "too many function inputs", -1, 0, -1)
	}
	size, err := intArray(dict.Get("Size"), inputs)
	if err != nil {
		return nil, err
	}
	for _, n := range size {
		if n <= 0 {
			return nil, errs.Wrap(errs.ErrCorrupt, "FUNC-SIZE", "invalid sample size", -1, 0, -1)
		}
	}
	rng, err := floatArray(dict.Get("Range"), -1, nil)
	if err != nil {
		return nil, err
	}
	if len(rng) == 0 || len(rng)%2 != 0 {
		return nil, errs.Wrap(errs.ErrCorrupt, "FUNC-RANGE", "invalid function range", -1, 0, -1)
	}
	outputs := len(rng) / 2
	encode, err := floatArray(dict.Get("Encode"), -1, nil)
	if err != nil {
		return nil, err
	}
	if len(encode) == 0 {
		encode = make([]float64, 2*inputs)
		for i := 0; i < inputs; i++ {
			encode[2*i] = 0
			encode[2*i+1] = float64(size[i] - 1)
		}
	}
	if len(encode) != 2*inputs {
		return nil, errs.Wrap(errs.ErrCorrupt, "FUNC-ENCODE", "encode length mismatch", -1, 0, -1)
	}
	decode, err := floatArray(dict.Get("Decode"), -1, nil)
	if err != nil {
		return nil, err
	}
	if len(decode) == 0 {
		decode = append([]float64(nil), rng...)
	}
	if len(decode) != 2*outputs {
		return nil, errs.Wrap(errs.ErrCorrupt, "FUNC-DECODE", "decode length mismatch", -1, 0, -1)
	}
	bits, ok := intValue(dict, "BitsPerSample")
	if !ok || bits <= 0 || bits > 32 {
		return nil, errs.Wrap(errs.ErrCorrupt, "FUNC-BITS", "invalid sample bit depth", -1, 0, -1)
	}
	data, err := file.DecodeStream(ctx, stm)
	if err != nil {
		return nil, err
	}
	total := outputs
	for _, n := range size {
		total *= n
	}
	if total <= 0 || int64(total)*int64(bits) > int64(len(data))*8 {
		return nil, errs.Wrap(errs.ErrCorrupt, "FUNC-DATA", "sampled function data is truncated", -1, 0, -1)
	}
	_ = tracker
	return &sampled{domain: domain, size: size, encode: encode, decode: decode, rng: rng, outputs: outputs, bits: bits, data: data}, nil
}

type sampled struct {
	domain  []float64
	size    []int
	encode  []float64
	decode  []float64
	rng     []float64
	outputs int
	bits    int
	data    []byte
}

func (f *sampled) Inputs() int  { return len(f.size) }
func (f *sampled) Outputs() int { return f.outputs }
func (f *sampled) Eval(in []float64) ([]float64, error) {
	if len(in) < len(f.size) {
		return nil, errs.Wrap(errs.ErrCorrupt, "FUNC-INPUT", "missing function input", -1, 0, -1)
	}
	coords := make([]float64, len(f.size))
	for i := range coords {
		x := norm(in[i], f.domain[2*i], f.domain[2*i+1])
		coords[i] = f.encode[2*i] + x*(f.encode[2*i+1]-f.encode[2*i])
		if coords[i] < 0 {
			coords[i] = 0
		}
		if max := float64(f.size[i] - 1); coords[i] > max {
			coords[i] = max
		}
	}
	out := make([]float64, f.outputs)
	for o := 0; o < f.outputs; o++ {
		out[o] = f.interp(coords, o)
	}
	clip(out, f.rng)
	return out, nil
}

// interp performs multilinear interpolation. The first input axis varies
// fastest in the sample array, as required by PDF.
func (f *sampled) interp(coords []float64, output int) float64 {
	n := len(coords)
	sum := 0.0
	for mask := 0; mask < 1<<n; mask++ {
		w := 1.0
		idx := 0
		stride := 1
		ok := true
		for i := 0; i < n; i++ {
			lo := int(math.Floor(coords[i]))
			if lo >= f.size[i]-1 {
				lo = f.size[i] - 1
			}
			frac := coords[i] - float64(lo)
			v := lo
			if mask&(1<<i) != 0 {
				if lo >= f.size[i]-1 {
					ok = false
					break
				}
				v = lo + 1
				w *= frac
			} else {
				w *= 1 - frac
			}
			idx += v * stride
			stride *= f.size[i]
		}
		if !ok || w == 0 {
			continue
		}
		sum += w * f.sampleComponent(idx, output)
	}
	return sum
}

func (f *sampled) sampleComponent(index, output int) float64 {
	i := index*f.outputs + output
	raw := readBits(f.data, i*f.bits, f.bits)
	max := float64(uint64(1)<<f.bits - 1)
	if f.bits == 32 {
		max = float64(math.MaxUint32)
	}
	v := float64(raw) / max
	lo, hi := f.decode[output*2], f.decode[output*2+1]
	return lo + v*(hi-lo)
}

func readBits(data []byte, bit, n int) uint32 {
	var v uint32
	for i := 0; i < n; i++ {
		pos := bit + i
		v = v<<1 | uint32((data[pos/8]>>uint(7-pos%8))&1)
	}
	return v
}

func loadPostScript(ctx context.Context, file *syntax.File, dict syntax.Dict, stm *syntax.Stream, tracker *budget.Tracker) (Function, error) {
	if stm == nil || file == nil {
		return nil, errs.Wrap(errs.ErrCorrupt, "FUNC-PS", "PostScript function stream missing", -1, 0, -1)
	}
	domain, err := floatArray(dict.Get("Domain"), -1, nil)
	if err != nil {
		return nil, err
	}
	if len(domain) == 0 || len(domain)%2 != 0 {
		return nil, errs.Wrap(errs.ErrCorrupt, "FUNC-DOMAIN", "invalid function domain", -1, 0, -1)
	}
	rng, err := floatArray(dict.Get("Range"), -1, nil)
	if err != nil {
		return nil, err
	}
	if len(rng) == 0 || len(rng)%2 != 0 {
		return nil, errs.Wrap(errs.ErrCorrupt, "FUNC-RANGE", "invalid function range", -1, 0, -1)
	}
	data, err := file.DecodeStream(ctx, stm)
	if err != nil {
		return nil, err
	}
	steps := int64(100000)
	if tracker != nil {
		steps = tracker.Budget().MaxFunctionSteps
	}
	prog, err := parsePostScript(data)
	if err != nil {
		return nil, err
	}
	return &postScript{domain: domain, rng: rng, prog: prog, steps: steps}, nil
}

type postScript struct {
	domain []float64
	rng    []float64
	prog   []psToken
	steps  int64
}

func (f *postScript) Inputs() int  { return len(f.domain) / 2 }
func (f *postScript) Outputs() int { return len(f.rng) / 2 }
func (f *postScript) Eval(in []float64) ([]float64, error) {
	if len(in) < f.Inputs() {
		return nil, errs.Wrap(errs.ErrCorrupt, "FUNC-INPUT", "missing function input", -1, 0, -1)
	}
	m := &psMachine{steps: f.steps}
	for i, x := range in {
		m.stack = append(m.stack, norm(x, f.domain[2*i], f.domain[2*i+1]))
	}
	if err := m.exec(f.prog); err != nil {
		return nil, err
	}
	want := f.Outputs()
	if len(m.stack) < want {
		return nil, errs.Wrap(errs.ErrCorrupt, "FUNC-PS-STACK", "function stack underflow", -1, 0, -1)
	}
	out := make([]float64, want)
	start := len(m.stack) - want
	for i := 0; i < want; i++ {
		v, ok := m.stack[start+i].(float64)
		if !ok {
			return nil, errs.Wrap(errs.ErrCorrupt, "FUNC-PS-TYPE", "function output is not a number", -1, 0, -1)
		}
		out[i] = v
	}
	clip(out, f.rng)
	return out, nil
}

func floatArray(o syntax.Object, want int, def []float64) ([]float64, error) {
	if o == nil {
		return def, nil
	}
	if _, ok := o.(syntax.Null); ok {
		return def, nil
	}
	arr, ok := o.(syntax.Array)
	if !ok {
		return nil, errs.Wrap(errs.ErrCorrupt, "FUNC-ARRAY", "function array expected", -1, 0, -1)
	}
	out := make([]float64, 0, len(arr))
	for _, v := range arr {
		n, ok := number(v)
		if !ok {
			return nil, errs.Wrap(errs.ErrCorrupt, "FUNC-NUMBER", "function number expected", -1, 0, -1)
		}
		out = append(out, n)
	}
	if want >= 0 && len(out) != want {
		return nil, errors.New("function array length mismatch")
	}
	return out, nil
}

func intArray(o syntax.Object, want int) ([]int, error) {
	arr, ok := o.(syntax.Array)
	if !ok {
		return nil, errs.Wrap(errs.ErrCorrupt, "FUNC-ARRAY", "function array expected", -1, 0, -1)
	}
	out := make([]int, 0, len(arr))
	for _, v := range arr {
		n, ok := number(v)
		if !ok {
			return nil, errs.Wrap(errs.ErrCorrupt, "FUNC-NUMBER", "function number expected", -1, 0, -1)
		}
		out = append(out, int(n))
	}
	if want >= 0 && len(out) != want {
		return nil, errors.New("function array length mismatch")
	}
	return out, nil
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

func intValue(d syntax.Dict, name string) (int, bool) {
	v, ok := d.IntValue(name)
	return int(v), ok
}

func norm(x, lo, hi float64) float64 {
	if hi <= lo {
		return 0
	}
	x = (x - lo) / (hi - lo)
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

func clip(vals, rng []float64) {
	for i := range vals {
		if len(rng) < (i+1)*2 {
			continue
		}
		lo, hi := rng[2*i], rng[2*i+1]
		if vals[i] < lo {
			vals[i] = lo
		}
		if vals[i] > hi {
			vals[i] = hi
		}
	}
}
