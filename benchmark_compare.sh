#!/usr/bin/env bash
# 스크립트 내에서 명령어가 실패하면 즉시 종료
set -e
# benchmark_compare.sh
# 이 스크립트를 프로젝트 루트 디렉토리에서 실행하세요.
# required : benchstat
# go install golang.org/x/perf/cmd/benchstat@latest

# 기존 결과 파일 삭제
rm -f preflight.txt preflight_combined.txt benchstat.txt

# BenchmarkPreFlight 실행 및 결과 저장, 벤치마크만 실행하도록 -run=^$ 옵션 추가
echo "Running BenchmarkPreFlight..."
go test -bench=BenchmarkPreFlight -run=^$ -benchmem ./... > preflight.txt

# BenchmarkPreFlight_old_250306 실행 및 결과 저장, 벤치마크만 실행하도록 -run=^$ 옵션 추가
echo "Running BenchmarkPreFlight_old_250306..."
go test -bench=BenchmarkPreFlight_old_250306 -run=^$ -benchmem ./... > preflight_combined.txt

# 두 결과 파일 비교
# TODO 일단 확인해보자.
# echo "Comparing benchmarks with benchstat..."
# benchstat preflight.txt preflight_combined.txt > benchstat.txt

# 결과 출력
# cat benchstat.txt
