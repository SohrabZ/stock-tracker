# Stock Tracker

[![CI](https://github.com/SohrabZ/stock-tracker/actions/workflows/ci.yml/badge.svg)](https://github.com/SohrabZ/stock-tracker/actions/workflows/ci.yml)
[![Go 1.25+](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE.md)

A command-line **stock trend monitor** written in Go. For each tracked symbol it
pulls 5-day hourly data from Yahoo Finance, computes trend/volume/signal metrics
and position P&L, detects changes since the last run, and prints a compact
markdown report. Built to run every 30 minutes from cron.

Zero external dependencies — pure Go standard library.

---

## Data source

Prices come from **Yahoo Finance's public chart API** — **no API key required**.

The only optional API is **OpenAI**: with `OPENAI_API_KEY` set, a short cached
`🧠 News:` line explaining the move is added on meaningful moves. It is gated and
cached so it costs at most one call per ticker per day (see **Cost controls**).

---

## Build

```bash
cd stock-tracker
go build -o stock-tracker .

cp .env.example .env                              # optional: OPENAI_API_KEY + cost controls
cp resources/positions.example.json resources/positions.json   # your holdings (git-ignored)
```

Requires Go 1.25+. `resources/positions.json` and `.env` are git-ignored (they
hold personal data / secrets); the committed `*.example.*` files are templates.

---

## Usage

```bash
./stock-tracker track                    # run ONE monitoring pass (the cron entry point)
./stock-tracker list                     # show tracked symbols
./stock-tracker add SLV                  # add symbol(s) to watch
./stock-tracker remove SLV               # remove symbol(s)
./stock-tracker position set SLV 970 59.55   # record a holding: 970 shares @ $59.55 avg
./stock-tracker position list            # show recorded positions
./stock-tracker position remove SLV      # delete a recorded position
./stock-tracker price SLV                # quick current-price check
./stock-tracker research SLV             # one-off AI news summary (needs OPENAI_API_KEY)
./stock-tracker track --loop --interval 1800   # run continuously (no cron)
```

`position set` records the holding **and** adds the symbol to the tracker list,
so it's monitored and shows P&L on the next run.

### Example output

```text
**SLV**  [🕐 After-hours]
*potential bottom forming / significant loss (-12.6%)*
• 💵 **$52.07**  |  🔴 Today **-3.43%**  |  🟢 5d **+4.02%**
• 📦 Position: 970 @ **$59.55**  |  Total: **$50,508**
• 🔴 💰 P&L: **$-7,256 (-12.6%)**  |  🎯 BE: **-12.6%**
• 📈 Status: **UP**
• 📊 Vol: **0.91x** (normal volume)  |  Recent: 719.9K  |  Avg: 792.2K
• 🔎 Why: bounced +4.6% off low
• 🧠 News: A steadier dollar and rising Fed rate-hike expectations pressured non-yielding silver, triggering profit-taking.

*Portfolio*: $50,508  |  P&L: **$-7,256 (-12.6%)**
```

- `🔎 Why` — the trend-analysis signals (free, always computed).
- `🧠 News` — optional LLM explanation, only on meaningful moves and cached once
  per ticker per day (omitted entirely when `OPENAI_API_KEY` is unset).

---

## What it computes

- **Trend**: linear-regression slope over the full period vs. the recent window
  → `is_downtrend` when the slope is negative and price is below the average.
- **Signals** (the `🔎 Why`): reversal signals (slope turning up, bounce off the
  low, volume spike with rising price, recovery above average) and acceleration
  signals (downtrend accelerating, volume spike with falling price, near the low).
- **Volume**: recent vs. average, with a descriptive label.
- **Sessions**: pre-market / regular / after-hours / closed, plus stale-data
  flagging when a session has no fresh trades.
- **Positions & P&L**: unrealized P&L, distance-to-breakeven, portfolio total —
  from `resources/positions.json`.
- **Change detection** (vs. the previous run's state): downtrend begin/end, new
  reversal/acceleration, and position alerts (loss crossed −$10K, exceeded −20%,
  near breakeven).

---

## Positions, P&L & Portfolio — where they come from

These lines come **entirely from `resources/positions.json`**. Nothing is
inferred and nothing is hardcoded. On every run, for each tracked symbol the
monitor looks that symbol up in the file:

- **Position found** → the alert gains three things:
  - `📦 Position: <qty> @ $<avg>  |  Total: $<qty × price>`
  - `💰 P&L: $<gain/loss> (<%>)  |  🎯 BE: <distance-to-breakeven %>`
  - the symbol is **included in the `*Portfolio*` total** at the bottom
- **Position not found** → the symbol shows price/trend/volume/why **only**;
  no P&L lines, and it is **excluded from the portfolio total**.

So a symbol reaches the P&L/portfolio output **only if you've recorded a
position for it**. That's why `SLV` shows P&L but a symbol you merely `add` for
watching does not.

### Adding a position

Easiest — use the CLI (records the holding and starts tracking the symbol):

```bash
./stock-tracker position set SLV 970 59.55   # SYM QTY AVG
./stock-tracker position list
./stock-tracker position remove SLV
```

Or edit `resources/positions.json` directly — the JSON key is the ticker
(uppercase):

```json
{
  "SLV": { "qty": 970, "avg": 59.55, "lots": [[457, 54.99], [513, 63.61]] }
}
```

- `qty` — total shares held.
- `avg` — blended average cost per share (drives P&L and breakeven).
- `lots` — optional record of the individual buys `[shares, price]`;
  informational, and only settable by editing the file.

Either way, changes take effect on the next run — no rebuild needed. The `*Portfolio*` line sums cost and market value across
only the tracked symbols that have a position here.

The flow, end to end:

```
tracker_list.json ─┐
                   ├─► for each symbol ─► Yahoo bars ─► analysis (trend/why)
positions.json ────┘                                        │
                        match symbol in positions.json ──────┤
                          found?  yes ─► add 📦/💰 lines + include in Portfolio
                                   no  ─► price/trend only, skip Portfolio
```

---

## Configuration

`resources/tracker_list.json` — symbols to monitor (manage via `add`/`remove`).

`resources/positions.json` — held positions (see the section above).

Environment (all optional, read from `.env`):

| Variable                  | Default        | Purpose                                             |
| ------------------------- | -------------- | --------------------------------------------------- |
| `OPENAI_API_KEY`          | *(unset)*      | Enables the `🧠 News:` line. Unset = no LLM, no cost.|
| `OPENAI_MODEL`            | `gpt-5.6-luna` | Model for the news line (cheapest tier).            |
| `AI_EXPLAIN`              | `true`         | Master switch for the news line.                    |
| `AI_EXPLAIN_MIN_MOVE_PCT` | `3.0`          | Min daily move magnitude to trigger a call.         |
| `AI_CACHE_REFRESH_PCT`    | `2.0`          | Re-generate only if the move shifts by this much.   |
| `ALERT_LOG_FILE`          | —              | (unused by monitor; reserved)                       |

## Caching & LLM cost control

The `🧠 News:` line is the **only** thing that costs money (one OpenAI call).
Because the monitor runs every 30 minutes (~25 times across the trading window),
calling the LLM every run would be wasteful — so it is gated **and cached**:

**Gating — is a call even wanted?**
1. **Off unless** `OPENAI_API_KEY` is set and `AI_EXPLAIN` ≠ `false`.
2. **Only on meaningful moves** — a trend-change alert fired, or the day's move
   magnitude ≥ `AI_EXPLAIN_MIN_MOVE_PCT` (default 3%).

**Cache — avoid paying twice for the same move.**
The explanation is stored in **`resources/explain_cache.json`**, keyed by ticker:

```json
{
  "SLV": {
    "date": "2026-07-23",
    "change_pct": -3.43,
    "why": "A steadier dollar and rising Fed rate-hike expectations…",
    "timestamp": "2026-07-23T13:26:25-07:00"
  }
}
```

On each run, if an explanation is wanted:
- **Cache hit** (same calendar day, move hasn't materially changed) → the cached
  `why` text is reused **for free**, no API call.
- **Cache miss / stale** (no entry today, *or* the move flipped sign, *or* it
  jumped ≥ `AI_CACHE_REFRESH_PCT`) → one call is made and the cache is updated.

**Net effect:** with a 30-minute cron over a market day, expect **~1 LLM call per
ticker per day**, not one per run. A verified example: the first run generated
the explanation (~20s), the next run returned it in ~2s from cache with identical
text. Delete `resources/explain_cache.json` to force a refresh.

---

## Cron setup

`run_tracker.sh` builds the binary if needed and runs one pass. Add to crontab
(`crontab -e`):

```cron
# Every 30 min, 1 AM–1 PM Pacific, Mon–Fri
*/30 1-13 * * 1-5 /Users/sohrabz/Codes/stock-tracker/run_tracker.sh >> /Users/sohrabz/Codes/stock-tracker/resources/cron.log 2>&1
```

The command exits non-zero if any ticker errored, so error-alerting can fire.

---

## Project structure

```
stock-tracker/
├── main.go                     # CLI entry point and command dispatch
├── go.mod
├── internal/
│   ├── config/                 # .env loader
│   ├── stock/                  # Yahoo Finance client (quotes + hourly bars)
│   ├── analyze/                # trend / volume / signal math
│   ├── positions/              # position & P&L math
│   ├── monitor/                # pass orchestration, formatting, change detection
│   ├── cache/                  # per-day AI explanation cache (cost control)
│   └── ai/                     # optional OpenAI news step
├── num/                    # shared numeric helpers
├── resources/                  # tracker_list, positions, monitor_state, caches
├── run_tracker.sh              # cron entry point
└── .env.example
```

---

## Contributing

Contributions are welcome — see [CONTRIBUTING.md](CONTRIBUTING.md) for the dev
setup, the pre-PR checklist (`build` / `vet` / `gofmt` / `test`), and the coding
guidelines (stdlib-only, tests beside code, DRY).

---

## License

Released under the MIT License — see [LICENSE.md](LICENSE.md).

**Not financial advice.** This tool reports public market data and technical
indicators for informational purposes only. Use at your own risk.
