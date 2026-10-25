package extension

// Status describes one provider feature. The values match the public capability
// vocabulary used by pdf.Capabilities and Document.Preflight.
type Status string

const (
	StatusSupported     Status = "supported"
	StatusApproximate   Status = "approximate"
	StatusExternal      Status = "external"
	StatusUnsupported   Status = "unsupported"
	StatusUnknown       Status = "unknown"
	StatusNotScanned    Status = "not-scanned"
	StatusNotApplicable Status = "not-applicable"
)

// Determinism describes whether a provider produces identical output on every
// supported platform.
type Determinism string

const (
	Deterministic     Determinism = "deterministic"
	PlatformDependent Determinism = "platform-dependent"
	ProviderDependent Determinism = "provider-dependent"
)

// ProviderInfo identifies one provider. Version must identify the actual
// implementation, including a pinned upstream revision when applicable.
type ProviderInfo struct {
	Name        string
	Version     string
	License     string
	Determinism Determinism
}

// Feature is one capability advertised by a provider. Reason is required for
// approximate, external, unsupported, unknown and not-scanned features.
type Feature struct {
	Name   string
	Status Status
	Reason string
}

// Provider is implemented by image, colour, font and future adapters.
type Provider interface {
	Info() ProviderInfo
	Features() []Feature
}
