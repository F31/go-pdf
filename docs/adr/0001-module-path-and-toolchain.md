# ADR-0001: Module path and toolchain

- Status: accepted
- Date: 2026-10-21
- Resolves: design appendix B Q5

## Decision

- Module path: `github.com/F31/go-pdf`.
- Repository: `https://github.com/F31/go-pdf`.
- Minimum supported Go version: 1.23.
- CI tests the minimum version and the current stable version.
- G0 has no external module dependencies. Only the standard library and
  `golang.org/x/*` may be added, subject to an ADR and license review.

## Rationale

The repository now exists under the F31 GitHub account, so the module path no
longer needs a placeholder. Go 1.23 provides the language and standard-library
features used by G0 while remaining recent enough for the supported toolchain
window.
