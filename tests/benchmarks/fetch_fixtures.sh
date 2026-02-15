#!/usr/bin/env bash
# fetch_fixtures.sh — download FRED JSON fixtures for benchmarks
#
# Usage:
#   FRED_API_KEY=your_key ./fetch_fixtures.sh
#   # or if already set in environment:
#   ./fetch_fixtures.sh
#
# Run this once from the repo root or from tests/benchmarks/.
# Commit the resulting fixtures/ JSON files — benchmarks run offline after that.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FIXTURE_DIR="${SCRIPT_DIR}/fixtures"
BASE_URL="https://api.stlouisfed.org/fred"

if [[ -z "${FRED_API_KEY:-}" ]]; then
  echo "Error: FRED_API_KEY is not set." >&2
  echo "  export FRED_API_KEY=your_key" >&2
  echo "  or: FRED_API_KEY=your_key ./fetch_fixtures.sh" >&2
  exit 1
fi

mkdir -p "${FIXTURE_DIR}"

fetch() {
  local name="$1"
  local url="$2"
  local out="${FIXTURE_DIR}/${name}.json"
  echo "  fetching ${name}..."
  curl -sf "${url}&api_key=${FRED_API_KEY}&file_type=json" \
    | python3 -m json.tool --compact \
    > "${out}"
  local lines
  lines=$(wc -c < "${out}")
  echo "    → ${out} (${lines} bytes)"
}

echo "==> Fetching observation fixtures..."

# GDP quarterly (~300 observations, 1947–present)
fetch "gdp_obs" \
  "${BASE_URL}/series/observations?series_id=GDP"

# CPIAUCSL monthly (~930 observations, 1947–present) — large payload
fetch "cpiaucsl_obs" \
  "${BASE_URL}/series/observations?series_id=CPIAUCSL"

# UNRATE monthly (~920 observations, 1948–present) — contains no missing values
fetch "unrate_obs" \
  "${BASE_URL}/series/observations?series_id=UNRATE"

# FEDFUNDS monthly (~830 observations, 1954–present)
fetch "fedfunds_obs" \
  "${BASE_URL}/series/observations?series_id=FEDFUNDS"

echo ""
echo "==> Fetching series metadata fixtures..."

# Batch metadata for 10 well-known series (simulates a search result payload)
for id in GDP CPIAUCSL UNRATE FEDFUNDS DGS10 M2SL PAYEMS INDPRO HOUST RSXFS; do
  fetch "meta_${id}" \
    "${BASE_URL}/series?series_id=${id}"
done

echo ""
echo "==> Done. Fixtures written to: ${FIXTURE_DIR}"
echo ""
echo "Fixture summary:"
ls -lh "${FIXTURE_DIR}"/*.json | awk '{print "  " $5 "\t" $9}'
echo ""
echo "Commit these files to lock in the benchmark dataset:"
echo "  git add tests/benchmarks/fixtures/"
echo "  git commit -m 'bench: add FRED JSON fixtures for json v1/v2 benchmarks'"
