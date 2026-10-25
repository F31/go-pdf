package pdf

import "fmt"

// Profile is a product capability profile from ADR-0012. Profiles describe
// release promises and provider requirements, not different parsers.
type Profile string

const (
	// ProfileCore covers parsing, decryption, interpretation, IR, text and
	// document semantics with a pure-Go, CGO-free build.
	ProfileCore Profile = "core"
	// ProfilePreview adds the screen-oriented Go rasterizer. It does not claim
	// print colour or complete native codec coverage.
	ProfilePreview Profile = "preview"
	// ProfileExtended adds explicitly selected native, CGO or external
	// providers for high-fidelity codecs, colour, output, editing and signing.
	ProfileExtended Profile = "extended"
)

// Valid reports whether p is a known profile.
func (p Profile) Valid() bool {
	switch p {
	case ProfileCore, ProfilePreview, ProfileExtended:
		return true
	default:
		return false
	}
}

// ParseProfile parses a profile name.
func ParseProfile(s string) (Profile, error) {
	p := Profile(s)
	if !p.Valid() {
		return "", fmt.Errorf("pdf: unknown profile %q", s)
	}
	return p, nil
}

// CapabilityStatus describes the support level of one feature.
type CapabilityStatus string

const (
	CapabilitySupported     CapabilityStatus = "supported"
	CapabilityApproximate   CapabilityStatus = "approximate"
	CapabilityExternal      CapabilityStatus = "external"
	CapabilityUnsupported   CapabilityStatus = "unsupported"
	CapabilityUnknown       CapabilityStatus = "unknown"
	CapabilityNotScanned    CapabilityStatus = "not-scanned"
	CapabilityNotApplicable CapabilityStatus = "not-applicable"
)

// Capability is one line in the public capability report.
type Capability struct {
	Area    string
	Feature string
	Status  CapabilityStatus
	Note    string
}

// Capabilities reports the Preview profile surface. It is a static release
// contract, not a per-document result.
func Capabilities() []Capability {
	return capabilityList()
}

// CapabilitiesFor returns the static capabilities promised by p. The Extended
// catalogue is the same as Preview until a provider is explicitly configured;
// per-document and per-renderer reports carry provider-specific status.
func CapabilitiesFor(p Profile) ([]Capability, error) {
	if !p.Valid() {
		return nil, fmt.Errorf("pdf: unknown profile %q", p)
	}
	all := capabilityList()
	if p != ProfileCore {
		return all, nil
	}
	out := make([]Capability, 0, len(all))
	for _, c := range all {
		switch c.Area {
		case "render", "color":
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

func capabilityList() []Capability {
	return []Capability{
		{"parse", "xref, object streams, repair", CapabilitySupported, ""},
		{"parse", "Flate/LZW/ASCII/RunLength/Predictor", CapabilitySupported, ""},
		{"parse", "CCITT G3 1-D/G4, DCT", CapabilitySupported, ""},
		{"parse", "CCITT G3 2-D, JBIG2/JPX core", CapabilityUnsupported, "Registry can inject external decoders"},
		{"parse", "indirect and Indexed image color spaces", CapabilitySupported, "DeviceGray/RGB/CMYK and ICCBased bases; image streams use decoded-size limits"},
		{"text", "Type0/CID, ToUnicode, CJK CMaps", CapabilitySupported, ""},
		{"text", "ActualText, MCID/StructRef", CapabilitySupported, ""},
		{"text", "word/line/block index", CapabilitySupported, "column-aware/tagged order; RTL/mixed; NFC/NFKC; rotated lines/Char.Quad/CharQuads; vertical RTL/LTR columns, vertical Quad"},
		{"text", "vertical W2/DW2 positioning", CapabilitySupported, "full bidi remains unsupported"},
		{"color", "ICCBased matrix/TRC and LUT8/LUT16/mAB to sRGB", CapabilitySupported, "PCS D50 XYZ Bradford-adapted to D65; unsupported source profiles fall back"},
		{"color", "external ColorEngine bridge", CapabilitySupported, "extension.ColorEngine -> pdf.ColorManager with cached transforms; sRGB destination"},
		{"render", "paths, clips, images, masks", CapabilityApproximate, "screen sRGB; basic stroke/AA"},
		{"render", "axial/radial/function/mesh shading", CapabilityApproximate, "type 6/7 patch supported"},
		{"render", "soft masks and transparency groups", CapabilityApproximate, "soft-mask/group backdrop; item-level Gray/CMYK/ICC; isolated group blend; knockout element-separated; analytic 45 dB SMask/knockout gates; synthetic Poppler reference; corpus goldens pending"},
		{"render", "text modes 4-7 and clip intersection", CapabilityApproximate, "glyph outlines or em-box fallback; rotated text-matrix advances and bounds fixed; Type3 clipping approximate"},
		{"render", "text outline fallback", CapabilityApproximate, "built-in Go sans/mono bold/italic variants; serif and CJK substituted"},
		{"render", "system font discovery", CapabilityApproximate, "used by default for IncludeText when Fonts is unset; platform TTF/OTF/TTC/OTC, family/style and CJK locale matching; simple subsets without Unicode cmap rejected"},
		{"render", "streaming page render pool", CapabilitySupported, "RenderPagesEach bounds in-flight pages and releases each raster after callback"},
		{"render", "PatternType 1/2 fills", CapabilityApproximate, "tiling patterns and axial/radial shading patterns; mesh/function shading patterns are diagnosed"},
		{"render", "whole-page and region raster", CapabilityApproximate, "Preview is screen sRGB; print colour and vectors require Extended"},
		{"render", "bounded page executor and output lease", CapabilitySupported, "fixed/adaptive tile plans; no unified process envelope"},
		{"render", "fixed/adaptive tile streaming and assembly", CapabilitySupported, "callback-owned copies; no spatial index or shared group surfaces"},
		{"render", "optional content expressions", CapabilitySupported, "OCMD /P and /VE filtering; expressions not retained in IR"},
		{"document", "links, outlines, page labels", CapabilitySupported, ""},
		{"document", "annotations and AcroForm metadata", CapabilitySupported, "AP rendering in Preview; no AP generation or form editing"},
		{"document", "structure tree and MCID mapping", CapabilitySupported, "RoleMap, ClassMap, /A/class attributes, typed BBox/Table/List accessors, Page/Document.Tables with grid, headers, paragraphs, nested tables, cross-page ID merge, heuristic TablesMerged, repeated header dedup and span diagnostics; lightweight"},
		{"document", "embedded file attachments", CapabilitySupported, "list and read"},
		{"document", "XMP core properties", CapabilitySupported, "title/creator/producer/dates"},
		{"document", "preflight status and outcomes", CapabilitySupported, "scans all pages; render remains approximate"},
		{"resource", "live file, raster, decoded images, fonts and caches", CapabilitySupported, "scene cache leases pending"},
		{"parse", "mmap-backed OpenFile/Open", CapabilitySupported, "Unix mmap; streaming ReaderAt parser pending"},
		{"text", "text search", CapabilitySupported, "case/whole-word; hit bounds and per-line quads"},
		{"text", "selection and highlight quads", CapabilitySupported, "rune range; per-line quads; normalized text"},
		{"security", "Standard R2-R6 (RC4/AES-128/AES-256)", CapabilitySupported, "WithPassword; SASLprep mapping/NFKC"},
	}
}
