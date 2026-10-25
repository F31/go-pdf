package color

import (
	"encoding/binary"
	"errors"
	"math"
)

type iccLUT struct {
	in, out, grid int
	grids         []int
	input         [][]float64
	matrix        [9]float64
	clut          []float64
	output        [][]float64
	pcs           string
	white         [3]float64
	mab           bool
	aCurves       []*iccCurve
	mCurves       []*iccCurve
	bCurves       []*iccCurve
	matrixT       [3]float64
	hasMatrix     bool
}

func parseICCLUT(b []byte, pcs string, white [3]float64) (*iccLUT, error) {
	if len(b) < 48 {
		return nil, errors.New("icc: LUT too short")
	}
	typ := string(b[0:4])
	in, out := int(b[8]), int(b[9])
	if in < 1 || in > 4 || out != 3 {
		return nil, errors.New("icc: unsupported LUT shape")
	}
	l := &iccLUT{in: in, out: out, pcs: pcs, white: white}
	if typ == "mAB " {
		return parseMAB(b, l)
	}
	grid := int(b[10])
	if grid < 2 || grid > 255 {
		return nil, errors.New("icc: unsupported LUT grid")
	}
	l.grid = grid
	for i := 0; i < 9; i++ {
		l.matrix[i] = s15(b[12+i*4 : 16+i*4])
	}
	switch typ {
	case "mft1":
		return parseLUT8(b, l)
	case "mft2":
		return parseLUT16(b, l)
	case "mAB ":
		return parseMAB(b, l)
	}
	return nil, errors.New("icc: unsupported LUT type")
}

func parseMAB(b []byte, l *iccLUT) (*iccLUT, error) {
	if len(b) < 32 {
		return nil, errors.New("icc: truncated mAB")
	}
	l.mab = true
	offs := [5]int{
		int(binary.BigEndian.Uint32(b[12:16])),
		int(binary.BigEndian.Uint32(b[16:20])),
		int(binary.BigEndian.Uint32(b[20:24])),
		int(binary.BigEndian.Uint32(b[24:28])),
		int(binary.BigEndian.Uint32(b[28:32])),
	}
	var err error
	if offs[0] != 0 {
		if l.bCurves, err = parseCurveSet(b, offs[0], l.out); err != nil {
			return nil, err
		}
	}
	if offs[1] != 0 {
		if offs[1]+48 > len(b) {
			return nil, errors.New("icc: truncated mAB matrix")
		}
		for i := 0; i < 9; i++ {
			l.matrix[i] = s15(b[offs[1]+i*4 : offs[1]+i*4+4])
		}
		for i := 0; i < 3; i++ {
			l.matrixT[i] = s15(b[offs[1]+36+i*4 : offs[1]+40+i*4])
		}
		l.hasMatrix = true
	}
	if offs[2] != 0 {
		if l.mCurves, err = parseCurveSet(b, offs[2], l.out); err != nil {
			return nil, err
		}
	}
	if offs[3] != 0 {
		if err := parseMABCLUT(b, offs[3], l); err != nil {
			return nil, err
		}
	}
	if offs[4] != 0 {
		if l.aCurves, err = parseCurveSet(b, offs[4], l.in); err != nil {
			return nil, err
		}
	}
	return l, nil
}

func parseMABCLUT(b []byte, off int, l *iccLUT) error {
	if off < 0 || off+16 > len(b) {
		return errors.New("icc: truncated mAB CLUT")
	}
	l.grids = make([]int, l.in)
	grid := 1
	for i := 0; i < l.in; i++ {
		g := int(b[off+i])
		if g < 2 || g > 128 {
			return errors.New("icc: invalid mAB grid")
		}
		l.grids[i] = g
		grid *= g
	}
	if grid > 1<<24 {
		return errors.New("icc: mAB CLUT too large")
	}
	pos := off + 16
	need := grid * l.out * 2
	if pos+need > len(b) {
		return errors.New("icc: truncated mAB CLUT data")
	}
	l.clut = make([]float64, grid*l.out)
	for i := range l.clut {
		l.clut[i] = float64(binary.BigEndian.Uint16(b[pos:])) / 65535
		pos += 2
	}
	return nil
}

func parseCurveSet(b []byte, off, count int) ([]*iccCurve, error) {
	if off < 0 || count < 0 {
		return nil, errors.New("icc: bad curve offset")
	}
	out := make([]*iccCurve, count)
	pos := off
	for i := 0; i < count; i++ {
		if pos < 0 || pos+12 > len(b) {
			return nil, errors.New("icc: truncated curve")
		}
		size, ok := iccCurveSize(b[pos:])
		if !ok || pos+size > len(b) {
			return nil, errors.New("icc: invalid curve")
		}
		c, ok := parseICCCurve(b[pos : pos+size])
		if !ok {
			return nil, errors.New("icc: invalid curve")
		}
		out[i] = c
		pos += size
	}
	return out, nil
}

func iccCurveSize(b []byte) (int, bool) {
	if len(b) < 12 {
		return 0, false
	}
	switch string(b[0:4]) {
	case "curv":
		count := int(binary.BigEndian.Uint32(b[8:12]))
		if count == 0 {
			return 12, true
		}
		if count == 1 {
			return 14, true
		}
		if count < 0 || count > 1<<20 || count > (len(b)-12)/2 {
			return 0, false
		}
		return 12 + count*2, true
	case "para":
		typ := int(binary.BigEndian.Uint16(b[8:10]))
		if typ < 0 || typ > 4 {
			return 0, false
		}
		need := [5]int{1, 3, 4, 5, 7}[typ]
		return 12 + need*4, true
	}
	return 0, false
}

func parseLUT8(b []byte, l *iccLUT) (*iccLUT, error) {
	pos := 48
	inputLen := l.in * 256
	if pos+inputLen > len(b) {
		return nil, errors.New("icc: truncated LUT8 input")
	}
	l.input = make([][]float64, l.in)
	for i := range l.input {
		table := make([]float64, 256)
		for j := range table {
			table[j] = float64(b[pos+i*256+j]) / 255
		}
		l.input[i] = table
	}
	pos += inputLen
	clutLen := powInt(l.grid, l.in) * l.out
	if pos+clutLen > len(b) {
		return nil, errors.New("icc: truncated LUT8 CLUT")
	}
	l.clut = make([]float64, clutLen)
	for i := range l.clut {
		l.clut[i] = float64(b[pos+i]) / 255
	}
	pos += clutLen
	l.output = make([][]float64, l.out)
	for i := range l.output {
		if pos+256 > len(b) {
			return nil, errors.New("icc: truncated LUT8 output")
		}
		table := make([]float64, 256)
		for j := range table {
			table[j] = float64(b[pos+i*256+j]) / 255
		}
		l.output[i] = table
		pos += 256
	}
	return l, nil
}

func parseLUT16(b []byte, l *iccLUT) (*iccLUT, error) {
	if len(b) < 52 {
		return nil, errors.New("icc: truncated LUT16 header")
	}
	inEntries := int(binary.BigEndian.Uint16(b[48:50]))
	outEntries := int(binary.BigEndian.Uint16(b[50:52]))
	if inEntries < 2 || outEntries < 2 {
		return nil, errors.New("icc: invalid LUT16 entries")
	}
	pos := 52
	inputLen := l.in * inEntries * 2
	if pos+inputLen > len(b) {
		return nil, errors.New("icc: truncated LUT16 input")
	}
	l.input = make([][]float64, l.in)
	for i := range l.input {
		table := make([]float64, inEntries)
		for j := range table {
			table[j] = float64(binary.BigEndian.Uint16(b[pos:])) / 65535
			pos += 2
		}
		l.input[i] = table
	}
	clutLen := powInt(l.grid, l.in) * l.out
	if pos+clutLen*2 > len(b) {
		return nil, errors.New("icc: truncated LUT16 CLUT")
	}
	l.clut = make([]float64, clutLen)
	for i := range l.clut {
		l.clut[i] = float64(binary.BigEndian.Uint16(b[pos:])) / 65535
		pos += 2
	}
	l.output = make([][]float64, l.out)
	for i := range l.output {
		if pos+outEntries*2 > len(b) {
			return nil, errors.New("icc: truncated LUT16 output")
		}
		table := make([]float64, outEntries)
		for j := range table {
			table[j] = float64(binary.BigEndian.Uint16(b[pos:])) / 65535
			pos += 2
		}
		l.output[i] = table
	}
	return l, nil
}

func powInt(a, b int) int {
	n := 1
	for i := 0; i < b; i++ {
		n *= a
	}
	return n
}

func (l *iccLUT) eval(vals []float64) ([4]float64, bool, error) {
	out, err := l.evalRaw(vals)
	if err != nil {
		return [4]float64{}, false, err
	}
	return l.pcsRGBA(out), false, nil
}

func (l *iccLUT) evalRaw(vals []float64) ([]float64, error) {
	if l == nil || len(vals) < l.in {
		return nil, clrErr("CLR-COMPONENTS", "not enough color components")
	}
	in := make([]float64, l.in)
	if l.mab {
		if l.aCurves != nil {
			for i, c := range l.aCurves {
				in[i] = c.eval(vals[i])
			}
		} else {
			for i := 0; i < l.in; i++ {
				in[i] = clamp(vals[i])
			}
		}
	} else {
		for i := 0; i < l.in; i++ {
			in[i] = lutAt(l.input[i], vals[i])
		}
	}
	var out []float64
	if l.mab {
		if l.clut != nil {
			out = l.clutEval(in)
		} else {
			out = append([]float64(nil), in[:minInt(3, l.in)]...)
		}
		if l.mCurves != nil {
			for i, c := range l.mCurves {
				if i < len(out) {
					out[i] = c.eval(out[i])
				}
			}
		}
		if l.hasMatrix && len(out) >= 3 {
			var m [3]float64
			for i := 0; i < 3; i++ {
				m[i] = l.matrix[i*3]*out[0] + l.matrix[i*3+1]*out[1] + l.matrix[i*3+2]*out[2] + l.matrixT[i]
			}
			out = m[:]
		}
		if l.bCurves != nil {
			for i, c := range l.bCurves {
				if i < len(out) {
					out[i] = c.eval(out[i])
				}
			}
		}
	} else {
		if l.in == 3 {
			var m [3]float64
			for i := 0; i < 3; i++ {
				m[i] = l.matrix[i*3]*in[0] + l.matrix[i*3+1]*in[1] + l.matrix[i*3+2]*in[2]
				in[i] = clamp(m[i])
			}
		}
		out = l.clutEval(in)
		for i := range out {
			out[i] = lutAt(l.output[i], out[i])
		}
	}
	for i := range out {
		out[i] = clamp(out[i])
	}
	return out, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (l *iccLUT) clutEval(vals []float64) []float64 {
	n := l.in
	out := make([]float64, l.out)
	if n < 1 {
		return out
	}
	if len(l.grids) != n {
		l.grids = nil
	}
	coords := make([]float64, n)
	lo := make([]int, n)
	frac := make([]float64, n)
	for i := 0; i < n; i++ {
		grid := l.grid
		if len(l.grids) > 0 {
			grid = l.grids[i]
		}
		if grid < 2 {
			return out
		}
		x := clamp(vals[i]) * float64(grid-1)
		lo[i] = int(math.Floor(x))
		if lo[i] >= grid-1 {
			lo[i] = grid - 2
			frac[i] = 1
		} else {
			frac[i] = x - float64(lo[i])
		}
		coords[i] = frac[i]
	}
	corners := 1 << n
	for mask := 0; mask < corners; mask++ {
		idx := 0
		w := 1.0
		for i := 0; i < n; i++ {
			bit := (mask >> i) & 1
			grid := l.grid
			if len(l.grids) > 0 {
				grid = l.grids[i]
			}
			idx = idx*grid + lo[i] + bit
			if bit == 1 {
				w *= coords[i]
			} else {
				w *= 1 - coords[i]
			}
		}
		if w == 0 {
			continue
		}
		base := idx * l.out
		for o := 0; o < l.out; o++ {
			out[o] += l.clut[base+o] * w
		}
	}
	return out
}

func lutAt(table []float64, x float64) float64 {
	if len(table) == 0 {
		return clamp(x)
	}
	if len(table) == 1 {
		return table[0]
	}
	x = clamp(x)
	pos := x * float64(len(table)-1)
	i := int(pos)
	if i >= len(table)-1 {
		return table[len(table)-1]
	}
	f := pos - float64(i)
	return table[i]*(1-f) + table[i+1]*f
}

func (l *iccLUT) pcsRGBA(v []float64) [4]float64 {
	if l.pcs == "Lab " {
		L := clamp(v[0]) * 100
		a := clamp(v[1])*255 - 128
		b := clamp(v[2])*255 - 128
		fy := (L + 16) / 116
		fx := fy + a/500
		fz := fy - b/200
		xyz := [3]float64{
			labInv(fx) * l.white[0],
			labInv(fy) * l.white[1],
			labInv(fz) * l.white[2],
		}
		return iccXYZRGBA(xyz)
	}
	return iccXYZRGBA([3]float64{clamp(v[0]), clamp(v[1]), clamp(v[2])})
}
