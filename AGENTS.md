# Agent notes

## Commands

- Build: `CGO_ENABLED=0 go build ./...`
- Test: `go test ./...` and `go test -race ./...`
- Vet: `go vet ./...`
- Fuzz smoke: `go test ./internal/syntax -run=^$ -fuzz=FuzzParseObject -fuzztime=20s`
- Benchmarks: `go test ./... -run=^$ -bench=. -benchmem`
- Dependency policy: `./scripts/check-deps.sh`

## Performance baseline

- Poppler 24.02 local baseline: `docs/perf/poppler-24.02-baseline.md`.
  Compare only on the same host, corpus, DPI, background and output format.

## Repository rules

- Only the Go standard library and `golang.org/x/*` are allowed. A new
  dependency requires an ADR and a `THIRD_PARTY.md` entry.
- Public packages are the module root (`pdf`), `displaylist`, `render`, and `text`. Internal PDF
  objects must not appear in public signatures.
- Do not silently degrade. Every approximation or unsupported feature must
  return an appropriate error or record a diagnostic.
- Errors must not contain document text, metadata values, or stream contents.
- `displaylist.Version` is frozen at 1.0 for G0. Add fields only (Minor bump);
  removals or semantic changes require a Major bump and an ADR.

## Git

- Remote: `git@github.com:F31/go-pdf.git` using the SSH key configured for the
  go-pptx workflow.
- Never commit `openspec/` or `.opencode/`; both are ignored by `.gitignore`.
- The design document lives in `docs/` and is tracked.
- Do not commit generated corpus files containing confidential material.
