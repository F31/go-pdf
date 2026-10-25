package pdf

import (
	"context"
	"errors"
	"testing"

	"github.com/F31/go-pdf/extension"
)

type testColorEngine struct {
	created int
	closed  int
	lastSrc extension.ColorSpec
	err     error
}

func (e *testColorEngine) Info() extension.ProviderInfo {
	return extension.ProviderInfo{Name: "test-color", Version: "1", License: "test", Determinism: extension.Deterministic}
}

func (e *testColorEngine) Features() []extension.Feature { return nil }

func (e *testColorEngine) NewTransform(_ context.Context, src, dst extension.ColorSpec) (extension.ColorTransform, error) {
	e.created++
	e.lastSrc = src
	if e.err != nil {
		return nil, e.err
	}
	return &testColorTransform{engine: e, src: src, dst: dst}, nil
}

type testColorTransform struct {
	engine *testColorEngine
	src    extension.ColorSpec
	dst    extension.ColorSpec
}

func (t *testColorTransform) SourceComponents() int      { return t.src.Components }
func (t *testColorTransform) DestinationComponents() int { return t.dst.Components }
func (t *testColorTransform) Transform(_ context.Context, out, in []float32, pixels int) error {
	for i := 0; i < pixels; i++ {
		base := i * t.src.Components
		obase := i * t.dst.Components
		out[obase] = in[base]
		out[obase+1] = 0.5
		out[obase+2] = 1 - in[base]
	}
	return nil
}
func (t *testColorTransform) Close() error { t.engine.closed++; return nil }

func TestColorManagerFromEngine(t *testing.T) {
	engine := &testColorEngine{}
	mgr := NewColorManagerFromEngine(engine)
	if mgr == nil {
		t.Fatal("nil manager")
	}
	c, ok := mgr.(interface{ Close() error })
	if !ok {
		t.Fatal("manager does not expose Close")
	}
	got, approx, err := mgr.Convert(ColorSpaceInfo{Name: "DeviceCMYK", Components: 4}, []float64{0.25, 0.1, 0.2, 0.3})
	if err != nil || approx || got != [4]float64{0.25, 0.5, 0.75, 1} {
		t.Fatalf("convert = %v approx=%v err=%v", got, approx, err)
	}
	if engine.created != 1 || engine.lastSrc.Name != "DeviceCMYK" || engine.lastSrc.Components != 4 {
		t.Fatalf("engine = %#v", engine)
	}
	if _, _, err := mgr.Convert(ColorSpaceInfo{Name: "DeviceCMYK", Components: 4}, []float64{0.1, 0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	if engine.created != 1 {
		t.Fatalf("transform not cached: %d", engine.created)
	}
	if _, _, err := mgr.Convert(ColorSpaceInfo{Name: "ICCBased", Components: 3, ICCProfile: []byte("profile-1")}, []float64{0.2, 0.3, 0.4}); err != nil {
		t.Fatal(err)
	}
	if engine.created != 2 || engine.lastSrc.Name != "ICCBased" || string(engine.lastSrc.Data) != "profile-1" {
		t.Fatalf("icc engine = %#v", engine)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if engine.closed != 2 {
		t.Fatalf("closed = %d", engine.closed)
	}
	if _, _, err := mgr.Convert(ColorSpaceInfo{Name: "DeviceCMYK", Components: 4}, []float64{0, 0, 0, 0}); err == nil {
		t.Fatal("conversion after Close succeeded")
	}
}

func TestColorManagerFromEngineErrors(t *testing.T) {
	if NewColorManagerFromEngine(nil) != nil {
		t.Fatal("nil engine produced a manager")
	}
	engine := &testColorEngine{err: errors.New("no transform")}
	mgr := NewColorManagerFromEngine(engine)
	if _, _, err := mgr.Convert(ColorSpaceInfo{Name: "DeviceCMYK", Components: 4}, []float64{0, 0, 0, 0}); err == nil {
		t.Fatal("engine error was ignored")
	}
	if _, _, err := mgr.Convert(ColorSpaceInfo{Name: "DeviceCMYK", Components: 4}, []float64{0}); err == nil {
		t.Fatal("missing components were accepted")
	}
}
