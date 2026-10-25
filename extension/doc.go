// Package extension defines optional provider ports for capabilities that are
// not implemented by the core renderer or document model.
//
// The package is intentionally declarative: it contains neutral data types and
// interfaces only, never internal go-pdf types. Implementations live in native
// components, adapters or external processes and are selected explicitly by the
// caller. Providers must report their version, licence, determinism and feature
// status through ProviderInfo and Features.
//
// Registry is an immutable, explicitly configured catalogue. It rejects
// duplicate encodings and provider names rather than relying on registration
// order. Existing pdf.DecoderFactory, pdf.ColorManager and render.FontProvider
// remain supported. They will be bridged to these ports in a later minor
// release; this package does not change current behaviour.
//
// Raster, writer, signature and text-shaping ports are out of scope for the
// server document core. High-fidelity work is delegated to Extended process
// adapters such as Poppler, OpenJPEG or Cairo.
//
// The extension/legacy subpackage adapts the original pdf.DecoderFactory,
// pdf.ColorManager and render.FontProvider interfaces for migration.
//
// See ADR-0012 (Core/Preview/Extended profiles) and ADR-0013 (component
// provenance and licence policy).
package extension
