# Repository Guidelines

## Project Structure & Module Organization

The repository root is the public `gobus` package. Core contracts and dispatch behavior live in files such as
`contract.go`, `dispatch.go`, and `dispatch_result.go`; keep package tests beside them as `*_test.go`. Use
`internal/example/` for runnable integration examples and `internal/performance/` for comparison implementations and
benchmarks. Generated mocks live in `internal/mocks/` and should be regenerated from the `//go:generate` directive in
`contract.go`, not edited manually. `testdata/consumer/` is a separate Go module that verifies the exported API from a
consumer's perspective. Release-only helpers belong in `scripts/`.

## Build, Test, and Development Commands

- `go build ./...` compiles the library and internal examples.
- `go run ./internal/example/cmd` runs the example application.
- `go test ./...` is the quick package-level test loop; it does not cross into the nested consumer module.
- `make test-consumer` tests the checkout through the isolated consumer module.
- `make check` is the complete local gate: formatting, vet, lint, unit and repeated race tests, `checkptr`, consumer
  verification, and coverage. It runs `go fmt` and creates ignored coverage artifacts, so review the diff afterward.
- `make bench-all` runs allocation-aware benchmarks; use it for dispatch-path or performance-sensitive changes.

## Coding Style & Naming Conventions

Write idiomatic Go and let `go fmt` control tabs and alignment. Run `make fmt` before committing and `make lint` for the
repository's stricter `golangci-lint` rules, including import grouping and a 130-column limit. Use concise lowercase
package names, exported `PascalCase` identifiers with documentation comments, and descriptive test names such as
`TestBus_DispatchAsyncReturnsExactlyOneResult`. Preserve the root package as the supported public facade; avoid exposing
`internal/` implementations.

## Testing Guidelines

Tests use Go's standard `testing` package, generally from external `_test` packages. Name unit tests `Test...`,
benchmarks `Benchmark...`, and fuzz targets `Fuzz...`. Add concurrency and allocation assertions when changing
registration or dispatch behavior. The full gate enforces at least 90% coverage; public API changes must also pass the
isolated consumer test.

## Commit & Pull Request Guidelines

Git history uses short, direct subjects without Conventional Commit prefixes, for example `Update README.md` and
`Remove comparable in types`. Keep each commit focused. Pull requests should explain behavior and API impact, list
verification performed, and link relevant issues. Include benchmark results for hot-path changes, and update `README.md`
plus `MIGRATION.md` when consumer-facing behavior changes.
