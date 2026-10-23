package pdf

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

// Capabilities reports the current reader/extractor/renderer surface. It is a
// static release contract, not a per-document result.
func Capabilities() []Capability {
	return []Capability{
		{"parse", "xref, object streams, repair", CapabilitySupported, ""},
		{"parse", "Flate/LZW/ASCII/RunLength/Predictor", CapabilitySupported, ""},
		{"parse", "CCITT G3 1-D/G4, DCT", CapabilitySupported, ""},
		{"parse", "CCITT G3 2-D, JBIG2/JPX core", CapabilityUnsupported, "Registry can inject external decoders"},
		{"text", "Type0/CID, ToUnicode, CJK CMaps", CapabilitySupported, ""},
		{"text", "ActualText, MCID/StructRef", CapabilitySupported, ""},
		{"text", "word/line index", CapabilitySupported, "horizontal and vertical columns; bidi not"},
		{"text", "vertical W2/DW2 positioning", CapabilitySupported, "full bidi remains unsupported"},
		{"color", "ICCBased matrix/TRC and LUT8/LUT16/mAB", CapabilitySupported, "mBA and unsupported profiles fall back"},
		{"color", "ICC mBA/B2A0 parsing and eval", CapabilitySupported, "inverse output path not wired"},
		{"render", "paths, clips, images, masks", CapabilityApproximate, "basic stroke/AA"},
		{"render", "axial/radial/function/mesh shading", CapabilityApproximate, "type 6/7 patch supported"},
		{"render", "soft masks and transparency groups", CapabilityApproximate, "soft-mask/group backdrop; Gray/CMYK/ICC item-level; isolated group blend; knockout backdrop; knockout element-separated"},
		{"render", "whole-page and region raster", CapabilitySupported, "PNG/JPEG encoding"},
		{"document", "links, outlines, page labels", CapabilitySupported, ""},
		{"document", "annotations and AcroForm metadata", CapabilitySupported, "AP rendering; no AP generation"},
		{"document", "structure tree and MCID mapping", CapabilitySupported, "lightweight"},
		{"document", "embedded file attachments", CapabilitySupported, "list and read"},
		{"document", "XMP core properties", CapabilitySupported, "title/creator/producer/dates"},
		{"document", "preflight status and outcomes", CapabilitySupported, "scans all pages; render remains approximate"},
		{"resource", "live file, raster, decoded images, fonts and caches", CapabilitySupported, "scene cache leases pending"},
		{"parse", "mmap-backed OpenFile/Open", CapabilitySupported, "Unix mmap; streaming ReaderAt parser pending"},
		{"render", "bounded page executor and output lease", CapabilitySupported, "fixed tiles; adaptive planner pending"},
		{"render", "fixed/adaptive tile streaming and assembly", CapabilitySupported, "callback-owned copies"},
		{"render", "optional content expressions", CapabilitySupported, "OCMD /P and /VE"},
		{"text", "text search", CapabilitySupported, "case/whole-word; hit bounds"},
		{"security", "Standard R2-R6 (RC4/AES-128/AES-256)", CapabilitySupported, "WithPassword; SASLprep mapping/NFKC"},
		{"edit", "write, page merge/split, signatures", CapabilityUnsupported, ""},
	}
}
