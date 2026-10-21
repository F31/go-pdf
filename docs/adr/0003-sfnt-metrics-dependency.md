# ADR-0003: Font program metrics via x/image/font/sfnt

- Status: accepted
- Date: 2026-10-21
- Resolves: design section 8.4 / FNT-05 / ADR-019

## Decision

Use `golang.org/x/image/font/sfnt` v0.39.0 to read TrueType/OpenType font
programs embedded as `/FontFile2` or `/FontFile3` with `/Subtype /OpenType`.
The same dependency supplies units-per-em, ascent/descent, cap height, x
height, and glyph advances used to populate `displaylist.FontMetrics` and
`FontMetrics.Advances`.

`golang.org/x/text` v0.36.0 is a transitive dependency of `x/image`; it is
listed in `THIRD_PARTY.md`. No other external module is added.

## Rationale

The design explicitly selects `x/image/font/sfnt` (ADR-019) instead of a
hand-written glyph/CFF parser. It is maintained by the Go project, passes the
repository dependency policy (`golang.org/x/*`), and is small enough to keep
the implementation and fuzz surface focused on PDF semantics.

## Consequences

- Font metrics are available only when the embedded program is an SFNT
  container. Raw Type1 (`/FontFile`), raw CFF (`/FontFile3` with `/Type1C` or
  `/CIDFontType0C`), Type3, and non-embedded fonts set
  `FontMetrics.Substituted=true`.
- `/CIDToGIDMap` is honored for CIDFontType2. For CIDFontType0 with no CMap
  path, advances fall back to Unicode-to-glyph lookup from `/ToUnicode`, or
  remain absent with a diagnostic.
- Adding or updating this dependency requires a license check and an update to
  `THIRD_PARTY.md`. v0.39.0 is the first release fixing GO-2026-4962 and
  requires Go 1.25; see ADR-0004.
