# Poppler corpus baseline (first-page sample)

Date: 2026-10-25
Host: Linux 6.6.87.2-microsoft-standard-WSL2 x86_64, 24 vCPU
Go: 1.25.0 linux/amd64
Poppler: 24.02.0

## Scope

- Private corpus: 57 PDFs under `docs/pdf/`.
- One page per file (page 1), 72 DPI, white background.
- Go render: `render.RenderPage`, `IncludeText=true`, `MaxPixels=64 MiB`.
- Fonts: the harness defaults to `render.BuiltinFonts()`. `POPPLER_CORPUS_FONT_DIR` pins explicit directories; `POPPLER_CORPUS_SYSTEM_FONTS=1` is for local host-dependent diagnostics only.
- Poppler: `pdftoppm -f N -l N -r 72 -png -singlefile -cropbox -hide-annotations`.
- Comparison: `internal/quality.Compare` (PSNR, SSIM, CIEDE2000).

Run:

```bash
POPPLER_CORPUS=1 go test ./render -run TestPopplerCorpusQuality -v -timeout 20m
```

Optional variables:

- `POPPLER_CORPUS_PAGES` (default 1): pages per file.
- `POPPLER_CORPUS_DPI` (default 72).
- `POPPLER_CORPUS_MIN_PSNR` (default 20).
- `POPPLER_CORPUS_MIN_SSIM` (default 0.85).
- `POPPLER_CORPUS_GATE=1`: fail when fallback rate exceeds the limit.
- `POPPLER_CORPUS_MAX_FALLBACK` (default 0.20).
- `POPPLER_CORPUS_FONT_DIR=/path1:/path2`: explicit font directories for reproducible CJK runs.
- `POPPLER_CORPUS_SYSTEM_FONTS=1`: use host fonts; reports are host-dependent.

## P0 reproducible sample (built-in fonts)

This is the current fixed-font baseline. It is directly comparable only with runs that
use the same built-in fonts, DPI, background, ICC and annotation policy.

| Metric | Value |
| --- | --- |
| Files / pages | 57 / 57 |
| Compared | 57 |
| Open errors | 0 |
| Go render errors | 0 |
| Poppler errors | 0 |
| Dimension mismatches | 0 |
| PSNR p10 / p50 / p90 | 19.263 / 25.121 / 34.269 dB |
| SSIM p10 / p50 / p90 | 0.611 / 0.900 / 0.985 |
| Fallback (<20 dB or <0.85 SSIM) | 21/57 (36.8%) |

Built-in fonts lack CJK outlines, so the SSIM p10 is lower than the historical
system-font sample. Use `POPPLER_CORPUS_FONT_DIR` to pin a CJK-capable font
directory when measuring CJK quality.

## Historical system-font sample (pre-P0)

| Metric | Value |
| --- | --- |
| Files / pages | 57 / 57 |
| Compared | 57 |
| Open errors | 0 |
| Go render errors | 0 |
| Poppler errors | 0 |
| Dimension mismatches | 0 |
| PSNR p10 / p50 / p90 | 19.26 / 25.74 / 34.27 dB |
| SSIM p10 / p50 / p90 | 0.820 / 0.903 / 0.985 |
| Fallback (<20 dB or <0.85 SSIM) | 21/57 (36.8%) |

This table is retained for historical comparison; it used host system fonts and
is not directly comparable to the fixed-font P0 baseline.

## Corpus harness performance

The full-corpus script now schedules page chunks across workers instead of
sharding whole files, uses PPM references, and caches parsed ICC profiles.

- 57-page fixed-font sample, 8 workers: ~29 s (previously ~10 min with file sharding).
- Full corpus, 16 workers, 128-page chunks: 13m26s for 10,866 pages, 0 errors.
- Each job prefetches the next Poppler page batch while rendering the current one.
  At 16 workers the gain is small because the run is already CPU/IO bound.
- The same 6,866 pages previously required more than 1 hour in one file shard.
- Rendering metrics were bit-identical before/after the harness optimization.
- ICC profiles are parsed once per process (bounded cache); even-odd fills use
  scanline intersections; CIEDE2000 skips identical pixels and reuses Lab
  values for repeated 8-bit colors.

Runs are reproducible with:

```bash
POPPLER_CORPUS_SHARDS=16 ./scripts/corpus-poppler-full.sh
```

## Historical system-font full run (final8)

- Pages: 10,866
- Fallback (<20 dB or <0.85 SSIM): 6,850/10,866 (63.0%)
- PSNR p10 / p50 / p90: 22.59 / 25.08 / 31.20 dB
- SSIM p10 / p50 / p90: 0.795 / 0.837 / 0.903
- Go render errors: 0
- Poppler errors: 0
- Dimension mismatches: 0

Report: `/tmp/poppler-corpus-final8/poppler-corpus.csv`.

The Luminosity-SoftMask black-bar defect in
`应用上容器指南2022年版BoCloud博云.pdf` pages 13/18 is fixed: PSNR/SSIM moved
from 11.35/0.065 and 11.41/0.083 to 26.41/0.740 and 27.24/0.806. Hairline
strokes below one pixel are now drawn at a minimum one-pixel width, which
raised the construction-drawing pages substantially (for example p339 went
from 13.93/0.382 to 15.81/0.733). Parametric ICC TRCs now use the ICC parameter
order `g,a,b,c,d,e,f`; this fixed Display-P3 images (for example the 智算底座
p26 image went from 15.02/0.764 to 25.74/0.979). The remaining top defects are
mostly text-hinting differences in `灵雀云运维手册 p82` and tender documents,
plus still-light construction hatching.

## Font policy

The Preview product still uses installed system fonts by default for real
documents, then the built-in Go fallback. That default is host-dependent.
Reproducible quality gates use `render.BuiltinFonts()` or
`POPPLER_CORPUS_FONT_DIR` with explicit font files. The fixed-font P0 baseline
is documented in `docs/perf/p0-baseline.md`.

## Interpretation

This is a Preview-profile baseline, not a production-quality or Poppler-parity
result. The remaining gap is dominated by source-font shape/style differences,
hinting, anti-aliasing, colour management and the absence of a unified
production compositing path. The sample is useful as a regression floor and for
tracking changes; it must not be used to claim Poppler parity.

The run also exposed and closed a number of concrete issues:

- indirect image `/ColorSpace` references were ignored;
- indexed images were decoded as DeviceGray;
- legitimate highly compressible images hit the generic inflate-ratio guard;
- base-14/non-embedded text had no outline fallback;
- rotated text matrices advanced glyphs along the wrong axis;
- `Td`/`T*` line moves were applied in the wrong text-space order;
- text bounds used font size instead of the glyph matrix;
- ICC matrix/LUT PCS XYZ was treated as D65 instead of adapting D50 to D65;
- PatternType 2 shading patterns emitted an empty tiling pattern;
- tiling patterns only repeated along X because the tile origin was added twice;
- ExtGState `/SMask` was not propagated into tiling or shading pattern content;
- indirect Form `/Group` dictionaries were ignored, so transparency groups were flattened and parent alpha was lost.

G8 remains blocked on full source/output ICC characterization, print colour,
external codec registration, reference goldens and full-page parallel
rasterization. The external `ColorEngine` bridge and Indexed/Separation/DeviceN
manager propagation are in place for sRGB Preview output.
