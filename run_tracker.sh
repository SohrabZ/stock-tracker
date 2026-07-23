#!/usr/bin/env bash
# Cron entry point: runs one trend-monitoring pass and prints the report to
# stdout. Resolves its own directory so it works regardless of cron's working
# dir. Exits non-zero if any ticker errored, so error-alerting can fire.
#
# Suggested schedule: */30 1-13 * * 1-5 (every 30 min, 1 AM–1 PM PT, Mon–Fri)
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$DIR"

# Build once if the binary is missing (build errors go to stderr, not stdout).
if [ ! -x "$DIR/stock-tracker" ]; then
  go build -o stock-tracker . 1>&2
fi

exec "$DIR/stock-tracker" track
