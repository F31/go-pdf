package pdf

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"

	"github.com/F31/go-pdf/extension"
	pdfcolor "github.com/F31/go-pdf/internal/color"
)

// ColorSpaceInfo identifies the color space passed to a ColorManager.
type ColorSpaceInfo struct {
	Name       string
	Components int
	ICCProfile []byte
}

// ColorManager optionally converts PDF color components to sRGB. It is called
// for DeviceCMYK and ICCBased spaces. Returning an error aborts the operation;
// callers must not silently fall back.
type ColorManager interface {
	Convert(info ColorSpaceInfo, values []float64) ([4]float64, bool, error)
}

type colorManagerAdapter struct {
	manager ColorManager
}

func (a colorManagerAdapter) Convert(space string, profile []byte, n int, values []float64) ([4]float64, bool, error) {
	return a.manager.Convert(ColorSpaceInfo{Name: space, Components: n, ICCProfile: profile}, values)
}

func wrapColorManager(m ColorManager) pdfcolor.Manager {
	if m == nil {
		return nil
	}
	return colorManagerAdapter{manager: m}
}

type engineColorManager struct {
	engine extension.ColorEngine

	mu         sync.Mutex
	transforms map[string]*engineColorTransform
	closed     bool
}

type engineColorTransform struct {
	mu sync.Mutex
	t  extension.ColorTransform
}

// NewColorManagerFromEngine adapts an extension ColorEngine to the PDF
// ColorManager surface. DeviceCMYK and ICCBased samples are converted to
// DeviceRGB through the engine. The returned manager also implements
// io.Closer; callers should Close it to release engine transforms.
func NewColorManagerFromEngine(engine extension.ColorEngine) ColorManager {
	if engine == nil {
		return nil
	}
	return &engineColorManager{engine: engine, transforms: map[string]*engineColorTransform{}}
}

func (m *engineColorManager) Convert(info ColorSpaceInfo, values []float64) ([4]float64, bool, error) {
	if m == nil || m.engine == nil {
		return [4]float64{}, false, fmt.Errorf("pdf: color engine is nil")
	}
	src := extension.ColorSpec{Name: info.Name, Components: info.Components, Data: info.ICCProfile}
	if src.Components <= 0 {
		src.Components = len(values)
	}
	if len(values) < src.Components {
		return [4]float64{}, false, fmt.Errorf("pdf: not enough color components")
	}
	key := colorTransformKey(src)
	t, err := m.transform(key, src)
	if err != nil {
		return [4]float64{}, false, err
	}
	var in [16]float32
	if src.Components > len(in) {
		return [4]float64{}, false, fmt.Errorf("pdf: too many color components")
	}
	for i := 0; i < src.Components; i++ {
		in[i] = float32(values[i])
	}
	var out [3]float32
	t.mu.Lock()
	err = t.t.Transform(context.Background(), out[:], in[:src.Components], 1)
	t.mu.Unlock()
	if err != nil {
		return [4]float64{}, false, err
	}
	return [4]float64{float64(out[0]), float64(out[1]), float64(out[2]), 1}, false, nil
}

func (m *engineColorManager) transform(key string, src extension.ColorSpec) (*engineColorTransform, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, fmt.Errorf("pdf: color manager is closed")
	}
	if t := m.transforms[key]; t != nil {
		return t, nil
	}
	t, err := m.engine.NewTransform(context.Background(), src, extension.ColorSpec{Name: "DeviceRGB", Components: 3})
	if err != nil {
		return nil, err
	}
	if t == nil || t.SourceComponents() != src.Components || t.DestinationComponents() != 3 {
		if t != nil {
			_ = t.Close()
		}
		return nil, fmt.Errorf("pdf: color engine returned an incompatible transform")
	}
	wrapped := &engineColorTransform{t: t}
	m.transforms[key] = wrapped
	return wrapped, nil
}

func colorTransformKey(src extension.ColorSpec) string {
	h := sha256.Sum256(src.Data)
	return fmt.Sprintf("%s/%d/%x", src.Name, src.Components, h[:8])
}

// Close releases cached transforms. It is safe to call more than once.
func (m *engineColorManager) Close() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	m.closed = true
	var first error
	for _, t := range m.transforms {
		t.mu.Lock()
		if err := t.t.Close(); err != nil && first == nil {
			first = err
		}
		t.mu.Unlock()
	}
	m.transforms = nil
	return first
}
