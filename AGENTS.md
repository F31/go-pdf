# Agent notes

## Commands

- Build: `CGO_ENABLED=0 go build ./...`
- Test: `go test ./...` and `go test -race ./...`
- Vet: `go vet ./...`
- Fuzz smoke: `go test ./internal/syntax -run=^$ -fuzz=FuzzParseObject -fuzztime=20s`
- Poppler synthetic reference: `POPPLER_QUALITY=1 go test ./render -run TestPopplerReferenceQuality -v`
- Poppler corpus sample: `POPPLER_CORPUS=1 go test ./render -run TestPopplerCorpusQuality -v -timeout 20m`
- Poppler full-corpus report: `POPPLER_CORPUS=1 POPPLER_CORPUS_PAGES=0 POPPLER_CORPUS_REPORT=/tmp/poppler-corpus.csv go test ./render -run TestPopplerCorpusQuality -v -timeout 24h`
- Poppler full corpus: `POPPLER_CORPUS_SHARDS=16 ./scripts/corpus-poppler-full.sh` (page-chunk workers, PPM, writes `/tmp/poppler-corpus/poppler-corpus.csv`)
- Core/provider separation: `./scripts/check-core-deps.sh`
- Licence inventory: `./scripts/check-licenses.sh`
- Quality metadata: `./scripts/quality-metadata.sh`
- Cross-engine diagnostic: `./scripts/cross-engine.sh <pdf> [page]`
- English corpus fetch: `./scripts/fetch-english-corpus.sh /tmp/english-corpus`
- English text-position P95: `./scripts/english-text-position.sh /tmp/english-corpus`
- Performance gate: `./scripts/perf-gate.sh`
- Release checks: `./scripts/release-check.sh`
- Poppler reference cache: add `POPPLER_CORPUS_REF_CACHE=/tmp/poppler-ref` (cached PNGs; reuses Poppler output between runs)
- Preview quality fonts: default `render.BuiltinFonts()`; pin CJK fonts with `POPPLER_CORPUS_FONT_DIR=/path1:/path2`
- Quality layers and report boundaries: `docs/perf/quality-layers.md`
- Benchmarks: `go test ./... -run=^$ -bench=. -benchmem`
- Dependency policy: `./scripts/check-deps.sh`

## Performance baseline

- P0 capability/font baseline: `docs/perf/p0-baseline.md`.
- P1 Core stability baseline: `docs/perf/p1-baseline.md`.
- P2 Preview semantics baseline: `docs/perf/p2-baseline.md`.
- P3 Extended provider baseline: `docs/perf/p3-baseline.md`.
- P4 quality/release baseline: `docs/perf/p4-baseline.md`; release policy: `docs/release.md`.
- Poppler 24.02 local baseline: `docs/perf/poppler-24.02-baseline.md`.
  Compare only on the same host, corpus, DPI, background and output format.

## Repository rules

- Only the Go standard library and `golang.org/x/*` are allowed. A new
  dependency requires an ADR and a `THIRD_PARTY.md` entry.
- Public packages are the module root (`pdf`), `displaylist`, `render`, `text`, `extension`, `extension/legacy`, `extension/poppler`, `extension/cairo`, `extension/openjpeg`, `extension/jbig2`, and `extension/g32d`. Internal PDF
  objects must not appear in public signatures.
- Do not silently degrade. Every approximation or unsupported feature must
  return an appropriate error or record a diagnostic.
- Errors must not contain document text, metadata values, or stream contents.
- `displaylist.Version` is 2.0. Add fields only (Minor bump); removals or
  semantic changes require a Major bump and an ADR. The 1.0 contract is not
  preserved.

## Git

- Remote: `git@github.com:F31/go-pdf.git` using the SSH key configured for the
  go-pptx workflow.
- Never commit `openspec/` or `.opencode/`; both are ignored by `.gitignore`.
- The design document lives in `docs/` and is tracked.
- Do not commit generated corpus files containing confidential material.
