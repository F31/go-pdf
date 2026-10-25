# Poppler corpus baseline (first-page sample)

Date: 2026-10-24
Host: Linux 6.6.87.2-microsoft-standard-WSL2 x86_64, 24 vCPU
Go: 1.25.0 linux/amd64
Poppler: 24.02.0

## Scope

- Private corpus: 57 PDFs under `docs/pdf/`.
- One page per file (page 1), 72 DPI, white background.
- Go render: `render.RenderPage`, `IncludeText=true` with the built-in Go sans/mono fallback, `MaxPixels=64 MiB`.
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

## Result

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

## Corpus harness performance

The full-corpus script now schedules page chunks across workers instead of
sharding whole files, uses PPM references, and caches parsed ICC profiles.

- 57-page sample, 8 workers: ~27 s (previously ~10 min with file sharding).
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

## Full corpus result

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

## System-font default

`IncludeText` now uses installed system fonts by default, then the built-in Go
fallback. System-font availability differs by host; set `PageOptions.Fonts`
explicitly for host-independent output. The reproducible gate should pin an
explicit font provider.

## Interpretation

This is a Preview-profile baseline, not a G8 production-quality result. The
built-in Go Regular fallback improved the sample from 44 to 41 fallback pages,
with PSNR p90 rising from 27.22 to 29.21 dB and SSIM p50 from 0.786 to 0.812.
The remaining gap is dominated by source-font shape/style differences,
anti-aliasing, colour management and the absence of a unified production
compositing path. The
sample is useful as a regression floor and for tracking changes; it must not be
used to claim Poppler parity.

The run also exposed and closed three concrete issues:

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
