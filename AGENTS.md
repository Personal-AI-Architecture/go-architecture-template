# Go PAA Template Instructions

This directory is the Go implementation template for Personal AI Architecture.

- Keep PAA-facing routes, schemas, message shapes, and stream events aligned
  with `specs/` and `paa.lock.json`.
- Keep provider-specific details in `internal/adapters/`.
- Keep runtime configuration separate from request metadata.
- Run `go run ./cmd/validate` before changing the contract boundary.
- Run the shared validator with `paa.target.json` before declaring the template
  acceptable.
