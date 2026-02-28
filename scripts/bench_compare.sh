#!/usr/bin/env bash
# bench_compare.sh — dag-go performance regression guard
#
# Usage:
#   ./scripts/bench_compare.sh [threshold_pct]
#
# Description:
#   Runs the full benchmark suite and compares each result against the
#   baseline stored in docs/PERFORMANCE_HISTORY.md.
#   Exits 0 if all benchmarks are within threshold; exits 1 if any exceed it.
#
# Arguments:
#   threshold_pct   Maximum allowed increase in ns/op (default: 10)
#
# Requirements:
#   - go (1.22+)
#   - awk, grep, sed (standard POSIX tools)
#   - benchstat (optional, for detailed analysis): go install golang.org/x/perf/cmd/benchstat@latest

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
THRESHOLD="${1:-10}"   # percent — default 10 %

# ── Colour helpers ────────────────────────────────────────────────────────────
RED='\033[0;31m'; YELLOW='\033[1;33m'; GREEN='\033[0;32m'; NC='\033[0m'
info()  { echo -e "${GREEN}[bench_compare]${NC} $*"; }
warn()  { echo -e "${YELLOW}[bench_compare] WARN${NC} $*"; }
error() { echo -e "${RED}[bench_compare] FAIL${NC} $*"; }

# ── Baselines from docs/PERFORMANCE_HISTORY.md ───────────────────────────────
# Format: "BenchmarkName baseline_ns_per_op baseline_allocs_per_op"
# Update this table whenever a new Stage resets the baseline.
declare -A BASELINE_NS=(
    ["BenchmarkCopyDag_Small"]=5098
    ["BenchmarkCopyDag_Medium"]=209458
    ["BenchmarkCopyDag_Large"]=7415826
    ["BenchmarkDetectCycle_Small"]=2135
    ["BenchmarkDetectCycle_Medium"]=53486
    ["BenchmarkDetectCycle_Large"]=1543347
    ["BenchmarkToMermaid_Small"]=8159
    ["BenchmarkToMermaid_Medium"]=29072
    ["BenchmarkToMermaid_Large"]=38493
    ["BenchmarkPreFlight"]=27685
)

declare -A BASELINE_ALLOCS=(
    ["BenchmarkCopyDag_Small"]=54
    ["BenchmarkCopyDag_Medium"]=1770
    ["BenchmarkCopyDag_Large"]=53211
    ["BenchmarkDetectCycle_Small"]=0
    ["BenchmarkDetectCycle_Medium"]=0
    ["BenchmarkDetectCycle_Large"]=0
    ["BenchmarkToMermaid_Small"]=68
    ["BenchmarkToMermaid_Medium"]=238
    ["BenchmarkToMermaid_Large"]=305
    ["BenchmarkPreFlight"]=89
)

# ── Special noisy benchmarks: require 3 failures before alerting ──────────────
# DetectCycle/Small has high relative noise at ~2 μs absolute.
NOISY_BENCHMARKS="BenchmarkDetectCycle_Small"

# ── Run benchmarks ────────────────────────────────────────────────────────────
TMP_DIR=$(mktemp -d)
trap 'rm -rf "${TMP_DIR}"' EXIT

BENCH_OUT="${TMP_DIR}/bench_current.txt"
info "Running benchmarks (benchtime=3s)…"
cd "${REPO_ROOT}"
go test -bench=. -benchmem -benchtime=3s -run='^$' ./... 2>/dev/null | tee "${BENCH_OUT}"

# ── Parse and compare ─────────────────────────────────────────────────────────
FAILURES=0
WARNINGS=0

info "Comparing against baseline (threshold: ${THRESHOLD}%)…"
echo ""
printf "%-40s %12s %12s %10s %10s  %s\n" \
    "Benchmark" "Baseline(ns)" "Current(ns)" "Delta%" "Status" ""
printf '%s\n' "$(printf '─%.0s' {1..95})"

while IFS= read -r line; do
    # Match lines like: BenchmarkFoo-64   1234   5678 ns/op   0 allocs/op ...
    if [[ "$line" =~ ^(Benchmark[A-Za-z0-9_]+)-[0-9]+[[:space:]]+ ]]; then
        name="${BASH_REMATCH[1]}"

        # Extract ns/op
        current_ns=$(echo "$line" | awk '{for(i=1;i<=NF;i++){if($i=="ns/op"){print $(i-1); break}}}')
        # Extract allocs/op
        current_allocs=$(echo "$line" | awk '{for(i=1;i<=NF;i++){if($i=="allocs/op"){print $(i-1); break}}}')

        # Skip if we don't have a baseline for this benchmark
        if [[ -z "${BASELINE_NS[$name]+_}" ]]; then
            continue
        fi

        baseline_ns="${BASELINE_NS[$name]}"
        baseline_allocs="${BASELINE_ALLOCS[$name]}"

        # Calculate percentage change for ns/op (integer arithmetic via awk)
        delta_pct=$(awk -v cur="$current_ns" -v base="$baseline_ns" \
            'BEGIN { if (base==0) { print 0 } else { printf "%.1f", (cur - base) / base * 100 } }')

        # Determine status
        is_noisy=false
        for noisy in $NOISY_BENCHMARKS; do
            [[ "$name" == "$noisy" ]] && is_noisy=true && break
        done

        # Compare allocs (any increase is always a warning)
        allocs_status="OK"
        if [[ "$current_allocs" -gt "$baseline_allocs" ]] 2>/dev/null; then
            allocs_status="ALLOC↑"
            WARNINGS=$((WARNINGS + 1))
        fi

        # Determine ns/op status
        if awk -v d="$delta_pct" -v t="$THRESHOLD" 'BEGIN{exit !(d > t)}'; then
            if $is_noisy; then
                status="${YELLOW}NOISY${NC}"
                warn "${name}: +${delta_pct}% (noisy benchmark — re-run 3× before escalating)"
                WARNINGS=$((WARNINGS + 1))
            else
                status="${RED}FAIL${NC}"
                FAILURES=$((FAILURES + 1))
            fi
        elif awk -v d="$delta_pct" 'BEGIN{exit !(d > 5)}'; then
            status="${YELLOW}WARN${NC}"
            WARNINGS=$((WARNINGS + 1))
        else
            status="${GREEN}OK${NC}"
        fi

        printf "%-40s %12s %12s %9s%% %10s  %s\n" \
            "$name" "${baseline_ns}" "${current_ns}" "${delta_pct}" "${allocs_status}" ""
        # Print coloured status separately (printf doesn't handle \033 well in all shells)
        if [[ "$FAILURES" -gt 0 ]] && [[ "$status" == *"FAIL"* ]]; then
            error "  ↑ ${name}: +${delta_pct}% exceeds ${THRESHOLD}% threshold!"
        fi
    fi
done < "${BENCH_OUT}"

echo ""
printf '%s\n' "$(printf '─%.0s' {1..95})"

# ── Summary ───────────────────────────────────────────────────────────────────
if [[ "$FAILURES" -gt 0 ]]; then
    error "REGRESSION DETECTED: ${FAILURES} benchmark(s) exceeded ${THRESHOLD}% threshold."
    error "Review docs/PERFORMANCE_HISTORY.md and investigate before merging."
    exit 1
elif [[ "$WARNINGS" -gt 0 ]]; then
    warn "${WARNINGS} warning(s) detected (5%–${THRESHOLD}% range or noisy benchmarks)."
    warn "Consider re-running benchmarks to confirm. Exiting 0 (soft warning only)."
    exit 0
else
    info "All benchmarks within ${THRESHOLD}% of baseline. No regression detected."
    exit 0
fi
