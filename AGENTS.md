# Repository Guidelines

## Project Structure & Module Organization

The repository root is the primary public `gobus` package. Core contracts and synchronous dispatch behavior live in
`contract.go`, `dispatch.go`, `dispatch_result.go`, and `event.go`; keep package tests beside them as `*_test.go`.
`async/` is the supported optional public package for bounded asynchronous execution. Use `internal/example/` for
runnable integration examples and `internal/performance/` for comparison implementations and benchmarks. Core
generated mocks live in `internal/mocks/`; performance mocks are colocated with their comparison packages. Regenerate
all mocks through their owning `//go:generate` directives with `go generate ./...`, and do not edit generated files
manually. `testdata/consumer/` is a separate Go module that verifies the exported API from a consumer's perspective.
Release-only helpers belong in `scripts/`.

## Build, Test, and Development Commands

- `go build ./...` compiles the library and internal examples.
- `go run ./internal/example/cmd` runs the example application.
- `go test ./...` is the quick package-level test loop; it does not cross into the nested consumer module.
- `go generate ./...` regenerates all mocks and mutates generated files; run it before the final gate when contracts
  change and verify that the resulting diff is committed.
- `make test-consumer` tests the checkout through the isolated consumer module, which uses a local `replace` directive.
- `make check` is the maximal local gate: formatting, build, vet, lint, unit and repeated race tests, `checkptr`, local
  consumer verification, and coverage. It runs `go fmt` and creates ignored coverage artifacts, so review the diff
  afterward.
- `make test-consumer-release VERSION=vX.Y.Z` tests a published tag from a clean temporary module without a local
  `replace`; run it only after the tag has been published.
- `make bench-all` runs allocation-aware benchmarks; use it for dispatch-path or performance-sensitive changes.

## Coding Style & Naming Conventions

Write idiomatic Go and let `go fmt` control tabs and alignment. Run `make fmt` before committing and `make lint` for the
repository's stricter `golangci-lint` rules, including import grouping and a 130-column limit. Use concise lowercase
package names, exported `PascalCase` identifiers with documentation comments, and descriptive test names such as
`TestBus_DispatchAsyncReturnsExactlyOneResult`. Preserve the root package as the primary public facade and `async/` as
its supported optional runtime package; avoid exposing `internal/` implementations.

## Testing Guidelines

Tests use Go's standard `testing` package, generally from external `_test` packages. Name unit tests `Test...`,
benchmarks `Benchmark...`, and fuzz targets `Fuzz...`. Add concurrency and allocation assertions when changing registry,
dispatch, or publish hot paths; add lifecycle, backpressure, and shutdown coverage for managed async changes. The full
gate enforces at least 90% coverage. Public API changes must update and pass the isolated local consumer test; published
releases must also pass the clean release-consumer check.

## Commit & Pull Request Guidelines

Git history contains both direct subjects such as `Update README.md` and Conventional Commit subjects such as
`feat: add events and bounded async runtime`. Keep subjects short and direct; Conventional Commit prefixes are allowed
but not required. Keep each commit focused. Pull requests should explain behavior and API impact, list verification
performed, and link relevant issues. Include benchmark results for hot-path changes, and update `README.md` plus
`MIGRATION.md` when consumer-facing behavior changes.
