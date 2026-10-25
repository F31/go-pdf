package color

import (
	"context"
	"math"
	"testing"

	"github.com/F31/go-pdf/internal/syntax"
)

func load(t *testing.T, obj syntax.Object) Space {
	t.Helper()
	sp, err := Load(context.Background(), nil, obj, nil)
	if err != nil {
		t.Fatal(err)
	}
	return sp
}

func TestDeviceAndCalRGB(t *testing.T) {
	sp := load(t, syntax.Array{
		syntax.Name("CalRGB"),
		syntax.Dict{
			"WhitePoint": syntax.Array{syntax.Real(0.95047), syntax.Int(1), syntax.Real(1.08883)},
			"Matrix":     syntax.Array{syntax.Real(0.95047), syntax.Int(0), syntax.Int(0), syntax.Int(0), syntax.Int(1), syntax.Int(0), syntax.Int(0), syntax.Int(0), syntax.Real(1.08883)},
		},
	})
	rgba, approx, err := sp.RGBA([]float64{1, 1, 1})
	if err != nil || approx {
		t.Fatalf("rgba %v approx %v err %v", rgba, approx, err)
	}
	for _, v := range rgba[:3] {
		if math.Abs(v-1) > 1e-4 {
			t.Fatalf("white %v", rgba)
		}
	}
}

func TestLabWhite(t *testing.T) {
	sp := load(t, syntax.Array{
		syntax.Name("Lab"),
		syntax.Dict{
			"WhitePoint": syntax.Array{syntax.Real(0.9642), syntax.Int(1), syntax.Real(0.8249)},
		},
	})
	rgba, approx, err := sp.RGBA([]float64{100, 0, 0})
	if err != nil || approx {
		t.Fatalf("rgba %v approx %v err %v", rgba, approx, err)
	}
	for _, v := range rgba[:3] {
		if math.Abs(v-1) > 1e-3 {
			t.Fatalf("white %v", rgba)
		}
	}
}

func TestIndexed(t *testing.T) {
	sp := load(t, syntax.Array{
		syntax.Name("Indexed"),
		syntax.Name("DeviceRGB"),
		syntax.Int(1),
		syntax.String{255, 0, 0, 0, 255, 0},
	})
	rgba, _, err := sp.RGBA([]float64{0})
	if err != nil || rgba[0] != 1 || rgba[1] != 0 {
		t.Fatalf("red %v %v", rgba, err)
	}
	rgba, _, err = sp.RGBA([]float64{1})
	if err != nil || rgba[0] != 0 || rgba[1] != 1 {
		t.Fatalf("green %v %v", rgba, err)
	}
}

func TestSeparation(t *testing.T) {
	sp := load(t, syntax.Array{
		syntax.Name("Separation"),
		syntax.Name("Spot"),
		syntax.Name("DeviceGray"),
		syntax.Dict{
			"FunctionType": syntax.Int(2),
			"Domain":       syntax.Array{syntax.Int(0), syntax.Int(1)},
			"C0":           syntax.Array{syntax.Int(0)},
			"C1":           syntax.Array{syntax.Int(1)},
			"N":            syntax.Int(1),
		},
	})
	rgba, _, err := sp.RGBA([]float64{0.5})
	if err != nil || math.Abs(rgba[0]-0.5) > 1e-9 || rgba[0] != rgba[1] {
		t.Fatalf("gray %v %v", rgba, err)
	}
}

type recordingManager struct {
	space  string
	n      int
	values []float64
	calls  int
}

func (m *recordingManager) Convert(space string, profile []byte, n int, values []float64) ([4]float64, bool, error) {
	m.calls++
	m.space, m.n, m.values = space, n, append([]float64(nil), values...)
	return [4]float64{0, 1, 0, 1}, false, nil
}

func TestManagerPropagatesThroughIndexedAndTint(t *testing.T) {
	m := &recordingManager{}
	indexed, err := Load(context.Background(), nil, syntax.Array{
		syntax.Name("Indexed"), syntax.Name("DeviceCMYK"), syntax.Int(0), syntax.String{0, 0, 0, 0},
	}, nil, m)
	if err != nil {
		t.Fatal(err)
	}
	if rgba, _, err := indexed.RGBA([]float64{0}); err != nil || rgba != [4]float64{0, 1, 0, 1} {
		t.Fatalf("indexed rgba=%v err=%v", rgba, err)
	}
	if m.calls != 1 || m.space != "DeviceCMYK" || m.n != 4 {
		t.Fatalf("indexed manager call=%d space=%s n=%d", m.calls, m.space, m.n)
	}

	sep := syntax.Array{
		syntax.Name("Separation"), syntax.Name("Spot"), syntax.Name("DeviceCMYK"),
		syntax.Dict{
			"FunctionType": syntax.Int(2),
			"Domain":       syntax.Array{syntax.Int(0), syntax.Int(1)},
			"C0":           syntax.Array{syntax.Int(0), syntax.Int(0), syntax.Int(0), syntax.Int(0)},
			"C1":           syntax.Array{syntax.Int(1), syntax.Int(1), syntax.Int(1), syntax.Int(1)},
			"N":            syntax.Int(1),
		},
	}
	sp, err := Load(context.Background(), nil, sep, nil, m)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := sp.RGBA([]float64{0.5}); err != nil {
		t.Fatal(err)
	}
	if m.calls != 2 || len(m.values) != 4 {
		t.Fatalf("separation manager call=%d values=%v", m.calls, m.values)
	}

}
