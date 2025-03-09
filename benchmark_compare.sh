#!/usr/bin/env bash
set -ex
# benchmark_compare.sh
# 이 스크립트를 프로젝트 루트 디렉토리에서 실행하세요.
# Required: benchstat (go install golang.org/x/perf/cmd/benchstat@latest)

RESULT_DIR="bench_results"
mkdir -p "$RESULT_DIR"
rm -f "$RESULT_DIR"/preflight.txt "$RESULT_DIR"/preflight_old_250306.txt "$RESULT_DIR"/benchstat.txt

# 환경변수를 설정하여 컬러 출력이 없도록 함 (TERM=dumb) TODO 이거 살펴보자.
export TERM=dumb

echo "Running BenchmarkPreFlight..."
go test -bench=^BenchmarkPreFlight$ -run=^$ -benchmem -count=10 ./... > "$RESULT_DIR"/preflight.txt

echo "Running BenchmarkPreFlight_old_250306..."
go test -bench=^BenchmarkPreFlight_old_250306$ -run=^$ -benchmem -count=10 ./... > "$RESULT_DIR"/preflight_old_250306.txt

echo "Comparing benchmarks with benchstat..."
benchstat "$RESULT_DIR"/preflight.txt "$RESULT_DIR"/preflight_old_250306.txt > "$RESULT_DIR"/benchstat.txt

cat "$RESULT_DIR"/benchstat.txt
