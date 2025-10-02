# Repository Guidelines

## Project Structure & Module Organization
Source lives in `internal/crawler/crawler.go`, which houses the concurrent crawling engine, and the CLI entry point is `cmd/linkcheck/main.go`. Keep additional packages under `internal/` for shared logic and prefer new binaries under `cmd/`. Runtime artifacts such as the `.linkcheck-cache.json` crawl cache should stay at the repo root and be gitignored if you introduce VCS metadata.

## Build, Test, and Development Commands
Use `go build ./...` to ensure every package compiles; add `GOCACHE=$PWD/.gocache` when working in sandboxed environments. Run `go run ./cmd/linkcheck -rpm 60 https://example.com` for quick manual checks, and prefer `go test ./...` for automated validation once you add tests. Always finish edits with `gofmt -w` on the touched Go files.

## Coding Style & Naming Conventions
Adhere to idiomatic Go style: exported types use PascalCase, locals use camelCase, and constants use mixedCaps or ALL_CAPS only for acronyms. Keep files ASCII, rely on Go’s standard library when possible, and route shared helpers through `internal/`. Format with `gofmt`; if you script tooling, ensure it preserves the current module layout and user-agent string defined in the crawler package.

## Testing Guidelines
Target new logic with table-driven tests in `internal/crawler`. Name tests `TestFeatureScenario` and add focused subtests for edge cases (rate limits, robots, cache reuse). Use `httptest` servers to emulate pages and robots.txt responses so tests stay hermetic. Gate new behaviors behind failing tests first when practical.

## Commit & Pull Request Guidelines
Write commits in the imperative mood (e.g., “Add robots parser”) and keep diffs focused; squash fixups before review. Pull requests should summarize the intent, enumerate notable limits or follow-up work, and include reproduction steps or sample commands. Link issues when applicable and call out impacts to the crawl cache or CLI flags so reviewers can verify backwards compatibility.

## Crawler Limits & Safety Defaults
The crawler enforces polite defaults: a 60 requests/minute cap, a 200-page internal follow limit, and an allowlist of HTML-style extensions. Respect these settings unless requirements demand otherwise, and pipe overrides through CLI flags so they remain discoverable. Robots.txt directives are honored by default; only bypass them with `-ignore-robots` during controlled testing, and document any intentional deviations.
