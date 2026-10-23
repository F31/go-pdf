package color

import (
	"context"
	"encoding/binary"
	"math"
	"testing"

	"github.com/F31/go-pdf/internal/syntax"
	"github.com/F31/go-pdf/internal/testkit"
)

func iccXYZ(v [3]float64) []byte {
	b := make([]byte, 20)
	copy(b, "XYZ ")
	for i, x := range v {
		binary.BigEndian.PutUint32(b[8+i*4:], uint32(int32(math.Round(x*65536))))
	}
	return b
}

func iccCurveBytes(gamma float64) []byte {
	b := make([]byte, 14)
	copy(b, "curv")
	binary.BigEndian.PutUint32(b[8:], 1)
	binary.BigEndian.PutUint16(b[12:], uint16(math.Round(gamma*256)))
	return b
}

func iccProfileBytes(colorSpace string, tags [][2]interface{}) []byte {
	header := 128 + 4 + len(tags)*12
	var body []byte
	for _, tag := range tags {
		body = append(body, tag[1].([]byte)...)
		for len(body)%4 != 0 {
			body = append(body, 0)
		}
	}
	data := make([]byte, header, header+len(body))
	copy(data[16:20], colorSpace)
	copy(data[20:24], "XYZ ")
	copy(data[36:40], "acsp")
	binary.BigEndian.PutUint32(data[0:], uint32(header+len(body)))
	binary.BigEndian.PutUint32(data[128:], uint32(len(tags)))
	offset := header
	for i, tag := range tags {
		base := 132 + i*12
		copy(data[base:base+4], tag[0].(string))
		binary.BigEndian.PutUint32(data[base+4:], uint32(offset))
		bodyLen := len(tag[1].([]byte))
		binary.BigEndian.PutUint32(data[base+8:], uint32(bodyLen))
		offset += bodyLen
		for offset%4 != 0 {
			offset++
		}
	}
	return append(data, body...)
}

func TestICCMatrixTRC(t *testing.T) {
	white := [3]float64{0.9642, 1, 0.8249}
	profile := iccProfileBytes("RGB ", [][2]interface{}{
		{"wtpt", iccXYZ(white)},
		{"rXYZ", iccXYZ([3]float64{0.4361, 0.2225, 0.0139})},
		{"gXYZ", iccXYZ([3]float64{0.3851, 0.7169, 0.0971})},
		{"bXYZ", iccXYZ([3]float64{0.1431, 0.0606, 0.7141})},
		{"rTRC", iccCurveBytes(1)},
		{"gTRC", iccCurveBytes(1)},
		{"bTRC", iccCurveBytes(1)},
	})
	p, err := parseICCProfile(profile, 3)
	if err != nil {
		t.Fatal(err)
	}
	rgba, approx, err := p.rgba([]float64{1, 1, 1})
	if err != nil || approx {
		t.Fatalf("white %v approx=%v err=%v", rgba, approx, err)
	}
	for _, v := range rgba[:3] {
		if math.Abs(v-1) > 2e-3 {
			t.Fatalf("white %v", rgba)
		}
	}
	rgba, _, err = p.rgba([]float64{1, 0, 0})
	if err != nil || rgba[0] < 0.9 || rgba[1] > 0.1 || rgba[2] > 0.1 {
		t.Fatalf("red %v err=%v", rgba, err)
	}
	tr, err := NewTransform(profile, 3)
	if err != nil {
		t.Fatal(err)
	}
	pcs, err := tr.ToLabPCS([]float64{1, 1, 1})
	if err != nil || math.Abs(pcs[0]-1) > 0.01 || math.Abs(pcs[1]-0.5) > 0.02 || math.Abs(pcs[2]-0.5) > 0.02 {
		t.Fatalf("matrix PCS %v err=%v", pcs, err)
	}
}

func TestICCGray(t *testing.T) {
	profile := iccProfileBytes("GRAY", [][2]interface{}{
		{"wtpt", iccXYZ([3]float64{0.9642, 1, 0.8249})},
		{"kTRC", iccCurveBytes(1)},
	})
	p, err := parseICCProfile(profile, 1)
	if err != nil {
		t.Fatal(err)
	}
	rgba, _, err := p.rgba([]float64{0.5})
	if err != nil || math.Abs(rgba[0]-srgb(0.5)) > 1e-3 || math.Abs(rgba[0]-rgba[1]) > 1e-3 || math.Abs(rgba[1]-rgba[2]) > 1e-3 {
		t.Fatalf("gray %v err=%v", rgba, err)
	}
}

func TestICCRejectBadProfile(t *testing.T) {
	if _, err := parseICCProfile([]byte("bad"), 3); err == nil {
		t.Fatal("expected error")
	}
}

func TestICCStreamLoad(t *testing.T) {
	profile := iccProfileBytes("RGB ", [][2]interface{}{
		{"wtpt", iccXYZ([3]float64{0.9642, 1, 0.8249})},
		{"rXYZ", iccXYZ([3]float64{0.4361, 0.2225, 0.0139})},
		{"gXYZ", iccXYZ([3]float64{0.3851, 0.7169, 0.0971})},
		{"bXYZ", iccXYZ([3]float64{0.1431, 0.0606, 0.7141})},
		{"rTRC", iccCurveBytes(1)},
		{"gTRC", iccCurveBytes(1)},
		{"bTRC", iccCurveBytes(1)},
	})
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Stream(4, "<< /N 3 >>", profile),
	}
	file, err := syntax.Open(context.Background(), testkit.Build(objs, "/Root 1 0 R"), syntax.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	sp, err := Load(context.Background(), file, syntax.Array{syntax.Name("ICCBased"), syntax.Ref{Num: 4}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if sp.Name() != "ICCBased" || sp.Approx() || sp.Components() != 3 {
		t.Fatalf("space %s approx=%v n=%d", sp.Name(), sp.Approx(), sp.Components())
	}
	rgba, approx, err := sp.RGBA([]float64{1, 1, 1})
	if err != nil || approx || math.Abs(rgba[0]-1) > 2e-3 {
		t.Fatalf("white %v approx=%v err=%v", rgba, approx, err)
	}
}

func makeCMYKLUT16() []byte {
	inputEntries, outputEntries := 2, 2
	grid, in, out := 2, 4, 3
	clutN := 1
	for i := 0; i < in; i++ {
		clutN *= grid
	}
	b := make([]byte, 52+in*inputEntries*2+clutN*out*2+out*outputEntries*2)
	copy(b, "mft2")
	b[8], b[9], b[10] = byte(in), byte(out), byte(grid)
	binary.BigEndian.PutUint16(b[48:], uint16(inputEntries))
	binary.BigEndian.PutUint16(b[50:], uint16(outputEntries))
	pos := 52
	for i := 0; i < in; i++ {
		for j := 0; j < inputEntries; j++ {
			binary.BigEndian.PutUint16(b[pos:], uint16(j*65535/(inputEntries-1)))
			pos += 2
		}
	}
	for i := 0; i < clutN; i++ {
		binary.BigEndian.PutUint16(b[pos:], 65535)
		binary.BigEndian.PutUint16(b[pos+2:], 32768)
		binary.BigEndian.PutUint16(b[pos+4:], 32768)
		pos += 6
	}
	for i := 0; i < out; i++ {
		for j := 0; j < outputEntries; j++ {
			binary.BigEndian.PutUint16(b[pos:], uint16(j*65535/(outputEntries-1)))
			pos += 2
		}
	}
	return b
}

func TestICCCMYKLUT16(t *testing.T) {
	profile := iccProfileBytes("CMYK", [][2]interface{}{
		{"wtpt", iccXYZ([3]float64{0.9642, 1, 0.8249})},
		{"A2B0", makeCMYKLUT16()},
	})
	copy(profile[20:24], "Lab ")
	if _, err := parseICCLUT(makeCMYKLUT16(), "Lab ", [3]float64{0.9642, 1, 0.8249}); err != nil {
		t.Fatal(err)
	}
	p, err := parseICCProfile(profile, 4)
	if err != nil {
		t.Fatal(err)
	}
	if p.components() != 4 {
		t.Logf("cs=%q pcs=%q profileCS=%q", p.colorSpace, p.pcs, string(profile[16:20]))
		t.Fatalf("components %d", p.components())
	}
	rgba, approx, err := p.rgba([]float64{0, 0, 0, 0})
	if err != nil || approx {
		t.Fatalf("rgba %v approx=%v err=%v", rgba, approx, err)
	}
	if rgba[0] < 0.9 || rgba[1] < 0.9 || rgba[2] < 0.9 {
		t.Fatalf("white %v", rgba)
	}
}

func TestICCCMYKStreamLoad(t *testing.T) {
	profile := iccProfileBytes("CMYK", [][2]interface{}{
		{"wtpt", iccXYZ([3]float64{0.9642, 1, 0.8249})},
		{"A2B0", makeCMYKLUT16()},
	})
	copy(profile[20:24], "Lab ")
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Stream(4, "<< /N 4 >>", profile),
	}
	file, err := syntax.Open(context.Background(), testkit.Build(objs, "/Root 1 0 R"), syntax.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	sp, err := Load(context.Background(), file, syntax.Array{syntax.Name("ICCBased"), syntax.Ref{Num: 4}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if sp.Components() != 4 || sp.Approx() {
		t.Fatalf("space n=%d approx=%v", sp.Components(), sp.Approx())
	}
	rgba, approx, err := sp.RGBA([]float64{0, 0, 0, 0})
	if err != nil || approx || rgba[0] < 0.9 {
		t.Fatalf("white %v approx=%v err=%v", rgba, approx, err)
	}
}

func makeCMYKMAB() []byte {
	b := make([]byte, 32+16+16*3*2)
	copy(b, "mAB ")
	b[8], b[9] = 4, 3
	binary.BigEndian.PutUint32(b[24:], 32)
	for i := 0; i < 4; i++ {
		b[32+i] = 2
	}
	pos := 48
	for i := 0; i < 16; i++ {
		binary.BigEndian.PutUint16(b[pos:], 65535)
		binary.BigEndian.PutUint16(b[pos+2:], 32768)
		binary.BigEndian.PutUint16(b[pos+4:], 32768)
		pos += 6
	}
	return b
}

func TestICCCMYKMAB(t *testing.T) {
	profile := iccProfileBytes("CMYK", [][2]interface{}{
		{"wtpt", iccXYZ([3]float64{0.9642, 1, 0.8249})},
		{"A2B0", makeCMYKMAB()},
	})
	copy(profile[20:24], "Lab ")
	p, err := parseICCProfile(profile, 4)
	if err != nil {
		t.Fatal(err)
	}
	if p.components() != 4 || p.lut == nil || !p.lut.mab {
		t.Fatalf("profile %#v", p)
	}
	rgba, _, err := p.rgba([]float64{0, 0, 0, 0})
	if err != nil || rgba[0] < 0.9 {
		t.Fatalf("white %v err=%v", rgba, err)
	}
}

func makeCMYKMBA() []byte {
	b := make([]byte, 32+16+8*4*2)
	copy(b, "mBA ")
	b[8], b[9] = 3, 4
	binary.BigEndian.PutUint32(b[24:], 32)
	for i := 0; i < 3; i++ {
		b[32+i] = 2
	}
	return b
}

func TestICCMBA(t *testing.T) {
	l, err := parseBToA(makeCMYKMBA())
	if err != nil {
		t.Fatal(err)
	}
	if l.in != 3 || l.out != 4 || len(l.grids) != 3 || len(l.clut) != 8*4 {
		t.Fatalf("btoa %#v", l)
	}
	out, err := l.eval([]float64{0, 0, 0})
	if err != nil || len(out) != 4 || out[0] != 0 {
		t.Fatalf("eval %v %v", out, err)
	}
}

func TestICCTransform(t *testing.T) {
	profile := iccProfileBytes("CMYK", [][2]interface{}{
		{"wtpt", iccXYZ([3]float64{0.9642, 1, 0.8249})},
		{"A2B0", makeCMYKLUT16()},
		{"B2A0", makeCMYKMBA()},
	})
	copy(profile[20:24], "Lab ")
	tr, err := NewTransform(profile, 4)
	if err != nil {
		t.Fatal(err)
	}
	dev, err := tr.FromSRGB([4]float64{1, 0, 0, 1})
	if err != nil || len(dev) != 4 {
		t.Fatalf("from %v %v", dev, err)
	}
	rgba, _, err := tr.ToSRGB(dev)
	if err != nil || rgba[0] < 0.9 {
		t.Fatalf("to %v %v", rgba, err)
	}
}

func TestICCDirectLabPCS(t *testing.T) {
	profile := iccProfileBytes("CMYK", [][2]interface{}{
		{"wtpt", iccXYZ([3]float64{0.9642, 1, 0.8249})},
		{"A2B0", makeCMYKLUT16()},
		{"B2A0", makeCMYKMBA()},
	})
	copy(profile[20:24], "Lab ")
	tr, err := NewTransform(profile, 4)
	if err != nil {
		t.Fatal(err)
	}
	pcs, err := tr.ToLabPCS([]float64{0, 0, 0, 0})
	if err != nil || len(pcs) != 3 || pcs[0] < 0.9 {
		t.Fatalf("pcs %v err=%v", pcs, err)
	}
	dev, err := tr.FromLabPCS(pcs)
	if err != nil || len(dev) != 4 {
		t.Fatalf("device %v err=%v", dev, err)
	}
}
