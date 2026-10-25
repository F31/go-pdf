package extension

import (
	"errors"
	"fmt"
	"sort"
)

// Registry is an immutable catalogue of explicitly selected providers. It may
// be shared by concurrent documents; provider sessions are still created per
// task. A Registry never falls back to an unregistered provider.
type Registry struct {
	images  map[ImageEncoding]ImageDecoderFactory
	colors  map[string]ColorEngine
	fonts   map[string]FontEngine
	encs    []ImageEncoding
	colorsN []string
	fontsN  []string
	infos   []ProviderInfo
}

// RegistryOption configures a Registry before it is frozen.
type RegistryOption func(*registryConfig) error

type registryConfig struct {
	images map[ImageEncoding]ImageDecoderFactory
	colors map[string]ColorEngine
	fonts  map[string]FontEngine
}

// WithImageFactory registers every encoding advertised by f. An encoding may
// have exactly one provider in a Registry.
func WithImageFactory(f ImageDecoderFactory) RegistryOption {
	return func(c *registryConfig) error {
		if f == nil {
			return errors.New("extension: nil image decoder factory")
		}
		info := f.Info()
		if err := validateInfo(info); err != nil {
			return err
		}
		encodings := f.Encodings()
		if len(encodings) == 0 {
			return fmt.Errorf("extension: image provider %q advertises no encodings", info.Name)
		}
		for _, enc := range encodings {
			if enc == "" {
				return fmt.Errorf("extension: image provider %q has an empty encoding", info.Name)
			}
			if existing, ok := c.images[enc]; ok {
				return fmt.Errorf("extension: encoding %q already registered by %q", enc, existing.Info().Name)
			}
			c.images[enc] = f
		}
		return nil
	}
}

// WithColorEngine registers a colour engine by its provider name.
func WithColorEngine(e ColorEngine) RegistryOption {
	return func(c *registryConfig) error {
		if e == nil {
			return errors.New("extension: nil colour engine")
		}
		info := e.Info()
		if err := validateInfo(info); err != nil {
			return err
		}
		if _, ok := c.colors[info.Name]; ok {
			return fmt.Errorf("extension: colour engine %q already registered", info.Name)
		}
		c.colors[info.Name] = e
		return nil
	}
}

// WithFontEngine registers a font engine by its provider name.
func WithFontEngine(e FontEngine) RegistryOption {
	return func(c *registryConfig) error {
		if e == nil {
			return errors.New("extension: nil font engine")
		}
		info := e.Info()
		if err := validateInfo(info); err != nil {
			return err
		}
		if _, ok := c.fonts[info.Name]; ok {
			return fmt.Errorf("extension: font engine %q already registered", info.Name)
		}
		c.fonts[info.Name] = e
		return nil
	}
}

// NewRegistry builds an immutable Registry. Repeated provider names or image
// encodings are rejected so selection never depends on registration order.
func NewRegistry(opts ...RegistryOption) (*Registry, error) {
	cfg := registryConfig{
		images: map[ImageEncoding]ImageDecoderFactory{},
		colors: map[string]ColorEngine{},
		fonts:  map[string]FontEngine{},
	}
	for _, opt := range opts {
		if opt == nil {
			return nil, errors.New("extension: nil registry option")
		}
		if err := opt(&cfg); err != nil {
			return nil, err
		}
	}
	r := &Registry{
		images: cfg.images,
		colors: cfg.colors,
		fonts:  cfg.fonts,
	}
	r.encs = make([]ImageEncoding, 0, len(cfg.images))
	for enc := range cfg.images {
		r.encs = append(r.encs, enc)
	}
	sort.Slice(r.encs, func(i, j int) bool { return r.encs[i] < r.encs[j] })
	r.colorsN = make([]string, 0, len(cfg.colors))
	for name := range cfg.colors {
		r.colorsN = append(r.colorsN, name)
	}
	sort.Strings(r.colorsN)
	r.fontsN = make([]string, 0, len(cfg.fonts))
	for name := range cfg.fonts {
		r.fontsN = append(r.fontsN, name)
	}
	sort.Strings(r.fontsN)
	infos, err := collectProviderInfo(cfg)
	if err != nil {
		return nil, err
	}
	r.infos = infos
	return r, nil
}

func validateInfo(info ProviderInfo) error {
	if info.Name == "" {
		return errors.New("extension: provider has an empty name")
	}
	if info.Version == "" {
		return fmt.Errorf("extension: provider %q has an empty version", info.Name)
	}
	if info.License == "" {
		return fmt.Errorf("extension: provider %q has an empty licence", info.Name)
	}
	if info.Determinism == "" {
		return fmt.Errorf("extension: provider %q has an empty determinism", info.Name)
	}
	return nil
}

func collectProviderInfo(cfg registryConfig) ([]ProviderInfo, error) {
	seen := map[string]ProviderInfo{}
	add := func(p Provider) error {
		info := p.Info()
		key := info.Name + "\x00" + info.Version
		if existing, ok := seen[key]; ok && existing != info {
			return fmt.Errorf("extension: provider %q version %q has conflicting metadata", info.Name, info.Version)
		}
		seen[key] = info
		return nil
	}
	for _, f := range cfg.images {
		if err := add(f); err != nil {
			return nil, err
		}
	}
	for _, e := range cfg.colors {
		if err := add(e); err != nil {
			return nil, err
		}
	}
	for _, e := range cfg.fonts {
		if err := add(e); err != nil {
			return nil, err
		}
	}
	out := make([]ProviderInfo, 0, len(seen))
	for _, info := range seen {
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].Version < out[j].Version
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// ImageFactory returns the factory registered for enc. It returns nil, false
// when no provider is registered; callers must report Unsupported/NotScanned.
func (r *Registry) ImageFactory(enc ImageEncoding) (ImageDecoderFactory, bool) {
	if r == nil {
		return nil, false
	}
	f, ok := r.images[enc]
	return f, ok
}

// Encodings returns registered image encodings in sorted order.
func (r *Registry) Encodings() []ImageEncoding {
	if r == nil {
		return nil
	}
	return append([]ImageEncoding(nil), r.encs...)
}

// ColorEngine returns the named colour engine.
func (r *Registry) ColorEngine(name string) (ColorEngine, bool) {
	if r == nil {
		return nil, false
	}
	e, ok := r.colors[name]
	return e, ok
}

// ColorEngineNames returns registered colour engine names in sorted order.
func (r *Registry) ColorEngineNames() []string {
	if r == nil {
		return nil
	}
	return append([]string(nil), r.colorsN...)
}

// FontEngine returns the named font engine.
func (r *Registry) FontEngine(name string) (FontEngine, bool) {
	if r == nil {
		return nil, false
	}
	e, ok := r.fonts[name]
	return e, ok
}

// FontEngineNames returns registered font engine names in sorted order.
func (r *Registry) FontEngineNames() []string {
	if r == nil {
		return nil
	}
	return append([]string(nil), r.fontsN...)
}

// Providers returns one entry per distinct provider name/version, sorted by
// name and version. The slice is a copy; Registry itself remains immutable.
func (r *Registry) Providers() []ProviderInfo {
	if r == nil {
		return nil
	}
	return append([]ProviderInfo(nil), r.infos...)
}
