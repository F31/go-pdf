package color

import (
	"encoding/binary"
	"errors"
	"math"
)

const (
	iccHeaderSize = 128
	iccTagSize    = 12
)

type iccProfile struct {
	colorSpace string
	white      [3]float64
	grayTRC    *iccCurve
	matrix     [9]float64
	trc        [3]*iccCurve
	lut        *iccLUT
	btoa       *iccBToA
	pcs        string
	isGray     bool
}

type iccCurve struct {
	lut   []float64
	gamma float64
	para  *iccPara
}

type iccPara struct {
	typ  int
	vals [7]float64
}

func parseICCProfile(data []byte, n int) (*iccProfile, error) {
	if len(data) < iccHeaderSize+4 {
		return nil, errors.New("icc: profile too short")
	}
	if string(data[36:40]) != "acsp" {
		return nil, errors.New("icc: bad signature")
	}
	cs := string(data[16:20])
	want := 0
	switch cs {
	case "GRAY":
		want = 1
	case "RGB ":
		want = 3
	case "CMYK":
		want = 4
	default:
		return nil, errors.New("icc: unsupported color space")
	}
	if n > 0 && n != want {
		return nil, errors.New("icc: N mismatch")
	}
	count := int(binary.BigEndian.Uint32(data[128:132]))
	if count < 0 || count > 4096 || iccHeaderSize+4+count*iccTagSize > len(data) {
		return nil, errors.New("icc: bad tag count")
	}
	tags := make(map[string][]byte, count)
	for i := 0; i < count; i++ {
		base := iccHeaderSize + 4 + i*iccTagSize
		sig := string(data[base : base+4])
		off := int(binary.BigEndian.Uint32(data[base+4 : base+8]))
		size := int(binary.BigEndian.Uint32(data[base+8 : base+12]))
		if off < 0 || size < 8 || off > len(data) || size > len(data)-off {
			continue
		}
		tags[sig] = data[off : off+size]
	}
	p := &iccProfile{colorSpace: cs, white: [3]float64{0.9642, 1, 0.8249}, pcs: string(data[20:24])}
	if tag, ok := tags["B2A0"]; ok {
		if btoa, err := parseBToA(tag); err == nil {
			p.btoa = btoa
		}
	} else if tag, ok := tags["B2A1"]; ok {
		if btoa, err := parseBToA(tag); err == nil {
			p.btoa = btoa
		}
	}
	if wp, ok := tags["wtpt"]; ok {
		if xyz, ok := parseICCXYZ(wp); ok {
			p.white = xyz
		}
	}
	if cs == "GRAY" {
		c, ok := parseICCCurve(tags["kTRC"])
		if !ok {
			return nil, errors.New("icc: missing gray TRC")
		}
		p.isGray = true
		p.grayTRC = c
		return p, nil
	}
	if cs == "CMYK" {
		if lut := parseProfileLUT(tags, p.pcs, p.white); lut != nil {
			p.lut = lut
			return p, nil
		}
		return nil, errors.New("icc: unsupported CMYK profile")
	}
	if cs != "RGB " {
		return nil, errors.New("icc: unsupported color space")
	}
	rx, ok1 := parseICCXYZ(tags["rXYZ"])
	gx, ok2 := parseICCXYZ(tags["gXYZ"])
	bx, ok3 := parseICCXYZ(tags["bXYZ"])
	if !ok1 || !ok2 || !ok3 {
		if lut := parseProfileLUT(tags, p.pcs, p.white); lut != nil {
			p.lut = lut
			return p, nil
		}
		return nil, errors.New("icc: missing colorants")
	}
	rt, ok1 := parseICCCurve(tags["rTRC"])
	gt, ok2 := parseICCCurve(tags["gTRC"])
	bt, ok3 := parseICCCurve(tags["bTRC"])
	if !ok1 || !ok2 || !ok3 {
		return nil, errors.New("icc: missing TRC")
	}
	p.matrix = [9]float64{
		rx[0], gx[0], bx[0],
		rx[1], gx[1], bx[1],
		rx[2], gx[2], bx[2],
	}
	p.trc = [3]*iccCurve{rt, gt, bt}
	return p, nil
}

func parseProfileLUT(tags map[string][]byte, pcs string, white [3]float64) *iccLUT {
	tag, ok := tags["A2B0"]
	if !ok {
		tag, ok = tags["A2B1"]
	}
	if !ok {
		return nil
	}
	lut, err := parseICCLUT(tag, pcs, white)
	if err != nil {
		return nil
	}
	return lut
}

func parseICCXYZ(b []byte) ([3]float64, bool) {
	if len(b) < 20 || string(b[0:4]) != "XYZ " {
		return [3]float64{}, false
	}
	return [3]float64{
		s15(b[8:12]), s15(b[12:16]), s15(b[16:20]),
	}, true
}

func parseICCCurve(b []byte) (*iccCurve, bool) {
	if len(b) < 12 {
		return nil, false
	}
	switch string(b[0:4]) {
	case "curv":
		count := int(binary.BigEndian.Uint32(b[8:12]))
		if count == 0 {
			return &iccCurve{gamma: 1}, true
		}
		if count == 1 {
			if len(b) < 14 {
				return nil, false
			}
			return &iccCurve{gamma: float64(binary.BigEndian.Uint16(b[12:14])) / 256}, true
		}
		if count < 0 || count > 1<<20 || len(b) < 12+count*2 {
			return nil, false
		}
		lut := make([]float64, count)
		for i := range lut {
			lut[i] = float64(binary.BigEndian.Uint16(b[12+i*2:14+i*2])) / 65535
		}
		return &iccCurve{lut: lut}, true
	case "para":
		if len(b) < 12 {
			return nil, false
		}
		typ := int(binary.BigEndian.Uint16(b[8:10]))
		if typ < 0 || typ > 4 {
			return nil, false
		}
		need := [5]int{1, 3, 4, 5, 7}[typ]
		if len(b) < 12+need*4 {
			return nil, false
		}
		var v [7]float64
		for i := 0; i < need; i++ {
			v[i] = s15(b[12+i*4 : 16+i*4])
		}
		return &iccCurve{para: &iccPara{typ: typ, vals: v}}, true
	}
	return nil, false
}

func s15(b []byte) float64 {
	return float64(int32(binary.BigEndian.Uint32(b))) / 65536
}

func (c *iccCurve) eval(x float64) float64 {
	if x < 0 {
		x = 0
	}
	if x > 1 {
		x = 1
	}
	if c == nil {
		return x
	}
	if c.para != nil {
		return c.para.eval(x)
	}
	if c.lut != nil {
		pos := x * float64(len(c.lut)-1)
		i := int(pos)
		if i >= len(c.lut)-1 {
			return c.lut[len(c.lut)-1]
		}
		f := pos - float64(i)
		return c.lut[i]*(1-f) + c.lut[i+1]*f
	}
	if c.gamma <= 0 {
		return x
	}
	return math.Pow(x, c.gamma)
}

func (p *iccPara) eval(x float64) float64 {
	v := p.vals
	switch p.typ {
	case 0:
		return math.Pow(x, v[0])
	case 1:
		if x >= -v[1]/v[0] {
			return math.Pow(v[0]*x+v[1], v[2])
		}
		return 0
	case 2:
		if x >= -v[1]/v[0] {
			return math.Pow(v[0]*x+v[1], v[2]) + v[3]
		}
		return v[3]
	case 3:
		if x >= v[4] {
			return math.Pow(v[0]*x+v[1], v[2])
		}
		return v[3] * x
	case 4:
		if x >= v[4] {
			return math.Pow(v[0]*x+v[1], v[2]) + v[3]
		}
		return v[3]*x + v[5]
	}
	return x
}

func (p *iccProfile) rgba(vals []float64) ([4]float64, bool, error) {
	if p.lut != nil {
		return p.lut.eval(vals)
	}
	if p.isGray {
		if len(vals) < 1 {
			return [4]float64{}, false, clrErr("CLR-COMPONENTS", "not enough color components")
		}
		l := p.grayTRC.eval(clamp(vals[0]))
		xyz := [3]float64{p.white[0] * l, p.white[1] * l, p.white[2] * l}
		return xyzRGBA(xyz, p.white), false, nil
	}
	if len(vals) < 3 {
		return [4]float64{}, false, clrErr("CLR-COMPONENTS", "not enough color components")
	}
	r := p.trc[0].eval(clamp(vals[0]))
	g := p.trc[1].eval(clamp(vals[1]))
	b := p.trc[2].eval(clamp(vals[2]))
	xyz := [3]float64{
		p.matrix[0]*r + p.matrix[1]*g + p.matrix[2]*b,
		p.matrix[3]*r + p.matrix[4]*g + p.matrix[5]*b,
		p.matrix[6]*r + p.matrix[7]*g + p.matrix[8]*b,
	}
	return xyzRGBA(xyz, p.white), false, nil
}
