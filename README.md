# linkcheck

`linkcheck` is a polite concurrent crawler for validating links in websites or web applications. It ships as a CLI that can crawl a start URL, follow internal links, and emit machine-readable healthcheck results suited for CI/CD pipelines.

## Features

- Concurrent crawling with request throttling and robots.txt support
- Healthcheck mode with JSON output and non-zero exit codes on failures
- YAML configuration with environment-specific presets and CLI overrides
- Crawl cache for faster re-runs and extension-based filtering

## Installation

```bash
go install ./cmd/linkcheck
```

The binary targets Go 1.25.1 or newer.

## Quick Start

```bash
linkcheck https://example.com
```

The default crawl honours robots.txt, follows up to 200 internal pages, and caps requests at 60 per minute.

## CLI Options

Key flags:

- `-config <path>`: load options from a YAML file (default `linkcheck.yaml` if present)
- `-print-config`: print the effective configuration as YAML and exit
- `-healthcheck`: run a single-page healthcheck and emit JSON (non-zero exit on failures)
- `-healthcheck-file <path>`: run healthcheck mode against every newline-separated URL in the given file
- `-healthcheck-interval <duration>`: rerun healthcheck mode on the given cadence (e.g. `30s`); exits on the first failure
- `-e`: validate external links in addition to internal ones
- `-workers`, `-timeout`, `-max-links`, `-rpm`, `-allow-ext`: fine-tune crawl behaviour
- `-cache`: set the cache file path (default `.linkcheck-cache.json`)

Run `linkcheck -h` to see the full flag list.

Flags always override YAML values when both are provided.

## YAML Configuration

`linkcheck` reads a YAML file when `-config` is provided (defaults to `linkcheck.yaml`). Every option has a sensible default so an empty or missing file is allowed. Example structure:

```yaml
start_url: https://example.com/
allow_external: false
workers: 8
timeout: 15s
max_links: 200
requests_per_minute: 60
allowed_extensions:
  - .html
  - .htm
ignore_robots: false
cache_path: .linkcheck-cache.json
healthcheck: false
healthcheck_file: ""
healthcheck_interval: 0s
```

Sample presets are included:

- `linkcheck.local.yaml` – development crawl against `http://localhost:8080`
- `linkcheck.web.yaml` – conservative crawl for public HTTPS sites

Use `linkcheck -config linkcheck.web.yaml -print-config` to inspect the resolved configuration.

## Healthcheck Mode

Healthcheck mode is designed for pipelines:

```bash
linkcheck -config linkcheck.web.yaml -healthcheck
```

- Emits a single JSON object describing status, HTTP code, duration, and collected errors
- Suppresses progress output and exits with `1` on any failure
- Stays within the configured rate limits and robots.txt policies

To validate multiple URLs in one run, provide a newline-separated list via `-healthcheck-file`:

```bash
linkcheck -healthcheck -healthcheck-file urls.txt
```

The CLI emits a JSON object with an overall `status` and a `results` array containing one entry per URL. The process exits with `0` only when every URL passes.

### Healthcheck File Format

- Plain text, one URL per line.
- Empty lines or lines starting with `#` are ignored.
- URLs without a scheme default to `https://` once normalized.

### Sample Batch Output

```json
{
  "status": "fail",
  "results": [
    {
      "url": "https://example.com",
      "status": "pass",
      "duration_ms": 124,
      "pages_visited": 1
    },
    {
      "url": "https://example.org/broken",
      "status": "fail",
      "duration_ms": 98,
      "pages_visited": 1,
      "errors": [
        {
          "message": "status 404"
        }
      ]
    }
  ]
}
```

### Continuous Healthchecks

Combine `-healthcheck` with `-healthcheck-interval` to keep probing a URL (or URL list) on a schedule:

```bash
linkcheck -healthcheck -healthcheck-interval 1m https://example.com
```

The command emits structured JSON after each run and sleeps for the requested duration. The process terminates immediately with exit code `1` when any run fails, making it suitable for watchdog scripts or container liveness probes.

The JSON output can be parsed to gate deployments, and failures provide explicit messages for troubleshooting.

## Development

- Build: `go build ./...`
- Tests: `GOCACHE=$PWD/.gocache go test ./...`
- Manual run: `go run ./cmd/linkcheck -rpm 60 https://example.com`

Always run `gofmt -w` on modified Go files before committing.

## License

MIT-style licensing is assumed; update this section if your project uses a different license.
