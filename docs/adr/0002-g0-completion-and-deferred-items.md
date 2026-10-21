# ADR-0002: G0 completion and deferred items

- Status: accepted
- Date: 2026-10-21

## Decision

G0 is considered functionally complete when the synthetic suite and the local
private corpus open and traverse pages, the display list version is frozen, and
the verification script passes except for tools unavailable in the environment.
The following items are explicitly deferred rather than silently approximated:

- Streaming/chunked input and zero-copy source windows.
- Full repair coverage for every malformed variant in design section 4.4.
- XMP field parsing and XMP-over-Info precedence (DOC-03).
- Encryption algorithms and password handling (CRY-01/CRY-02).
- DCT/CCITT/JBIG2/JPX decoding (G3) and image mask composition.
- Content interpretation, fonts, colors, images, and regional rendering (G1–G3b).

## Rationale

G0's purpose is a stable, testable foundation. Keeping the parser conservative
and returning `ErrUnsupported` for later phases avoids contract churn and makes
G1 work additive. Private corpus files stay outside the repository; only
synthetic fixtures and golden outputs are committed.

## Consequences

- Local corpus coverage is not enforced by CI; public corpus acquisition remains
  a release prerequisite.
- Benchmarks are recorded but the 5% blocking threshold stays advisory until a
  controlled runner is available.
- Any deferred item that changes public behavior must arrive through a new
  OpenSpec change.
