package diag

// G0 diagnostic codes. New codes must use one of the documented prefixes.
const (
	CodeSyntaxHeader     = "SYN-NO-HEADER"
	CodeSyntaxStartXref  = "SYN-NO-STARTXREF"
	CodeSyntaxXRef       = "SYN-XREF-OFFSET"
	CodeSyntaxLength     = "SYN-LEN-MISMATCH"
	CodeSyntaxNesting    = "BUD-NESTING-DEPTH"
	CodeSyntaxRepair     = "SYN-REPAIRED"
	CodeSyntaxMissingObj = "SYN-MISSING-OBJECT"
	CodeFilterUnknown    = "FLT-UNKNOWN-FILTER"
	CodeFilterNoDecoder  = "FLT-NO-DECODER"
	CodeFilterPredictor  = "FLT-PREDICTOR-PARAMS"
	CodeBudgetStream     = "BUD-STREAM-BYTES"
	CodeBudgetRatio      = "BUD-INFLATE-RATIO"
	CodeBudgetTotal      = "BUD-TOTAL-DECODED"
	CodeDocNoRoot        = "DOC-NO-ROOT"
	CodeDocPageRange     = "DOC-PAGE-RANGE"
	CodeDocVersion       = "DOC-VERSION-MISMATCH"
	CodeDocLinearized    = "DOC-LINEARIZED"
	CodeCryptoPassword   = "CRY-PASSWORD"
	CodeCryptoPubSec     = "CRY-PUBSEC"
)
