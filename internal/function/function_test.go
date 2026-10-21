package function

import (
	"context"
	"math"
	"testing"

	"github.com/F31/go-pdf/internal/budget"
	"github.com/F31/go-pdf/internal/syntax"
	"github.com/F31/go-pdf/internal/testkit"
)

func eval(t *testing.T, f Function, in ...float64) []float64 {
	t.Helper()
	out, err := f.Eval(in)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestExponential(t *testing.T) {
	f, err := Load(context.Background(), nil, syntax.Dict{
		"FunctionType": syntax.Int(2),
		"Domain":       syntax.Array{syntax.Int(0), syntax.Int(1)},
		"C0":           syntax.Array{syntax.Int(0)},
		"C1":           syntax.Array{syntax.Int(1)},
		"N":            syntax.Int(2),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := eval(t, f, 0.5)[0]; math.Abs(got-0.25) > 1e-9 {
		t.Fatalf("got %v", got)
	}
}

func TestStitching(t *testing.T) {
	f, err := Load(context.Background(), nil, syntax.Dict{
		"FunctionType": syntax.Int(3),
		"Domain":       syntax.Array{syntax.Int(0), syntax.Int(1)},
		"Range":        syntax.Array{syntax.Int(0), syntax.Int(1)},
		"Bounds":       syntax.Array{syntax.Real(0.5)},
		"Encode":       syntax.Array{syntax.Int(0), syntax.Real(0.5), syntax.Real(0.5), syntax.Int(1)},
		"Functions": syntax.Array{
			syntax.Dict{"FunctionType": syntax.Int(2), "Domain": syntax.Array{syntax.Int(0), syntax.Int(1)}, "C0": syntax.Array{syntax.Int(0)}, "C1": syntax.Array{syntax.Int(1)}},
			syntax.Dict{"FunctionType": syntax.Int(2), "Domain": syntax.Array{syntax.Int(0), syntax.Int(1)}, "C0": syntax.Array{syntax.Int(0)}, "C1": syntax.Array{syntax.Int(1)}},
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := eval(t, f, 0.25)[0]; math.Abs(got-0.25) > 1e-9 {
		t.Fatalf("low %v", got)
	}
	if got := eval(t, f, 0.75)[0]; math.Abs(got-0.75) > 1e-9 {
		t.Fatalf("high %v", got)
	}
}

func openFunction(t *testing.T, objs []testkit.Obj) Function {
	t.Helper()
	ctx := context.Background()
	b := budget.DefaultBudget
	file, err := syntax.Open(ctx, testkit.Build(objs, "/Root 1 0 R"), syntax.Options{Tracker: budget.NewTracker(&b)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	o, err := file.ResolveRef(ctx, syntax.Ref{Num: 2})
	if err != nil {
		t.Fatal(err)
	}
	f, err := Load(ctx, file, o, budget.NewTracker(&b))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestSampled2D(t *testing.T) {
	f := openFunction(t, []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog >>"),
		testkit.Stream(2, "<< /FunctionType 0 /Domain [0 1 0 1] /Range [0 1] /Size [2 2] /BitsPerSample 8 /Encode [0 1 0 1] /Decode [0 1] >>", []byte{0, 85, 170, 255}),
	})
	out := eval(t, f, 0.5, 0.5)
	if math.Abs(out[0]-0.5) > 1e-9 {
		t.Fatalf("got %v", out)
	}
	if got := eval(t, f, 0, 0)[0]; got != 0 {
		t.Fatalf("corner %v", got)
	}
	if got := eval(t, f, 1, 1)[0]; got != 1 {
		t.Fatalf("corner %v", got)
	}
}

func TestPostScript(t *testing.T) {
	f := openFunction(t, []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog >>"),
		testkit.Stream(2, "<< /FunctionType 4 /Domain [0 1] /Range [0 10] >>", []byte("{ 2 mul 1 add }")),
	})
	if got := eval(t, f, 0.5)[0]; math.Abs(got-2) > 1e-9 {
		t.Fatalf("got %v", got)
	}
}

func TestPostScriptControl(t *testing.T) {
	f := openFunction(t, []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog >>"),
		testkit.Stream(2, "<< /FunctionType 4 /Domain [0 1] /Range [0 1] >>", []byte("{ dup 0.5 gt { 1 exch sub } if }")),
	})
	if got := eval(t, f, 0.75)[0]; math.Abs(got-0.25) > 1e-9 {
		t.Fatalf("got %v", got)
	}
	if got := eval(t, f, 0.25)[0]; math.Abs(got-0.25) > 1e-9 {
		t.Fatalf("got %v", got)
	}
}

func FuzzPostScript(f *testing.F) {
	f.Add([]byte("{ dup mul }"))
	f.Add([]byte("0.5 2 mul 1 add"))
	f.Add([]byte("{ dup 0.5 gt { 1 exch sub } if }"))
	f.Add([]byte("{ 1 2 3 2 copy }"))
	f.Fuzz(func(t *testing.T, data []byte) {
		prog, err := parsePostScript(data)
		if err != nil {
			return
		}
		m := &psMachine{steps: 10000}
		_ = m.exec(prog)
	})
}
