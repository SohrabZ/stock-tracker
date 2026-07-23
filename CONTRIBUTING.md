# Contributing

Thanks for your interest in improving Stock Tracker! This is a small,
dependency-free Go project — contributions of all sizes are welcome.

## Development setup

```bash
git clone <your-fork-url>
cd stock-tracker
go build -o stock-tracker .

# local config (git-ignored)
cp .env.example .env
cp resources/positions.example.json resources/positions.json
```

Requires **Go 1.25+**. There are no external dependencies — only the standard
library.

## Before you open a PR

Please make sure all of these pass:

```bash
go build ./...     # compiles
go vet ./...       # static checks
gofmt -l .         # must print nothing (run `gofmt -w .` to fix)
go test ./...      # all tests green
```

A one-liner:

```bash
go build ./... && go vet ./... && test -z "$(gofmt -l .)" && go test ./...
```

## Coding guidelines

- **Formatting**: always `gofmt`. CI/reviewers will reject unformatted code.
- **No new dependencies** without discussion — staying stdlib-only is a design goal.
- **Tests live next to code** as `*_test.go` in the same package (Go convention),
  so they can exercise unexported functions. Add or update tests for any
  behavior change.
- **Keep it DRY**: shared math lives in `internal/num`; the Yahoo/OpenAI clients
  and formatting each have a single home. Prefer extending those over copying.
- **Comments** explain *why*, not *what*. Match the surrounding style.

## Project layout

| Package             | Responsibility                                         |
| ------------------- | ------------------------------------------------------ |
| `main`              | CLI entry point and command dispatch                   |
| `internal/config`   | `.env` loader                                          |
| `internal/stock`    | Yahoo Finance client (quotes + hourly bars)            |
| `internal/analyze`  | trend / volume / signal math                           |
| `internal/positions`| position & P&L math                                    |
| `internal/monitor`  | pass orchestration, formatting, change detection       |
| `internal/cache`    | per-day AI explanation cache (cost control)            |
| `internal/ai`       | optional OpenAI news step                              |
| `internal/num`      | shared numeric helpers                                 |

## Notes for contributors

- **Never commit secrets or personal data.** `.env` and `resources/positions.json`
  are git-ignored on purpose; edit the `*.example.*` templates instead.
- **Mind LLM cost.** The `internal/ai` calls are paid. Any change there must keep
  the gating + caching in `internal/monitor` intact (see the README's
  "Caching & LLM cost control" section).
- **Market data is public and unauthenticated** — be considerate of Yahoo's
  endpoint (the client already sets a timeout and a User-Agent).

## Reporting issues

Open an issue with: what you ran, what you expected, what happened, and the
relevant output (redact any personal position figures).
