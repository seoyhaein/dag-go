.PHONY: test lint lint-fix fmt bench bench-compare coverage

# 로컬 bin에 golangci-lint가 있으면 우선 사용, 없으면 PATH에서 탐색
GOLANGCI_LINT ?= ./bin/golangci-lint

# 회귀 감지 임계치 (기본값: 10%). 오버라이드: make bench-compare BENCH_THRESHOLD=15
BENCH_THRESHOLD ?= 10

test:
	go test -v -race -cover ./...

bench:
	go test -bench=. -benchmem -benchtime=3s -run=^$ ./...

# bench-compare: 현재 벤치마크 결과를 PERFORMANCE_HISTORY.md 기준선과 비교.
# 10% 이상 하락 시 비제로 exit code 반환.
bench-compare:
	@bash scripts/bench_compare.sh $(BENCH_THRESHOLD)

# coverage: 커버리지 프로파일 생성 및 함수별 상세 출력
coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

lint:
	$(GOLANGCI_LINT) run --config=.golangci.yml ./...

lint-fix:
	$(GOLANGCI_LINT) run --config=.golangci.yml --fix ./...

fmt:
	go fmt ./...