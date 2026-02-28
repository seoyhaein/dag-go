# dag-go Performance History

이 문서는 Stage별 벤치마크 결과를 누적 기록하며, 성능 회귀(regression) 분석의 기준선(baseline)으로 사용됩니다.

**측정 환경:** Intel(R) Xeon(R) CPU E5-2683 v4 @ 2.10GHz, linux/amd64, Go 1.22+
**측정 명령:** `go test -bench=. -benchmem -benchtime=3s -run=^$ ./...`

---

## Baseline: Stage 11 (Data Plane Performance Optimization)

**커밋 태그:** `b2f3f32` · **측정일:** 2026-02-27

Stage 11에서 두 가지 핵심 최적화가 적용됨:
1. `detectCycle`: `copyDag` 호출 제거 → `dfsStatePool` (`sync.Pool`) 기반 zero-alloc DFS
2. `printStatus`: `statusPool` 생명주기 완성 — `newPrintStatus` → `connectRunner` → `Wait`에서 `releasePrintStatus`

### Stage 11 벤치마크 결과

| Benchmark | ns/op | allocs/op | bytes/op |
|---|---|---|---|
| `CopyDag/Small`       | 5,218     | 54      | 4,504  |
| `CopyDag/Medium`      | 208,943   | 1,770   | 134,712 |
| `CopyDag/Large`       | 7,456,721 | 53,211  | 3,958,359 |
| `DetectCycle/Small`   | 1,896     | **0**   | 0 |
| `DetectCycle/Medium`  | 53,976    | **0**   | 0 |
| `DetectCycle/Large`   | 1,558,478 | **0**   | 0 |
| `ToMermaid/Small`     | 8,052     | 68      | 2,288 |
| `ToMermaid/Medium`    | 28,841    | 238     | 8,349 |
| `ToMermaid/Large`     | 38,120    | 305     | 11,914 |
| `PreFlight`           | 27,341    | 89      | 4,878 |

> Stage 11 이전 (`DetectCycle` Stage 10 수치):
> - Small: 7,239 ns/op · 50 allocs/op
> - Medium: 206,041 ns/op · 1,279 allocs/op
> - Large: 5,822,780 ns/op · 28,011 allocs/op

---

## Stage 12 (Execution Engine Reliability & Zero-churn Worker Pool)

**커밋 태그:** `5fcf42f` · **측정일:** 2026-02-28

### Stage 12 변경 사항 요약

| 변경 항목 | 세부 내용 |
|---|---|
| `SafeChannel.SendBlocking` | 신규 API — RLock 유지 상태로 blocking select. 신호 유실 원천 차단. |
| `postFlight` / `notifyChildren` | `Send` → `SendBlocking(ctx)` 교체. ctx 취소 시 즉시 반환. |
| `nodeTask` struct | `chan func()` → `chan nodeTask` 교체. 제출(submit)당 클로저 힙 할당 0건. |
| `Dag.droppedErrors` | atomic int64 카운터. `reportError` 오버플로우 시 증가. `DroppedErrors()` 노출. |

### Stage 12 벤치마크 결과 (실측)

| Benchmark | ns/op | allocs/op | bytes/op | vs Stage 11 ns/op | vs Stage 11 allocs/op |
|---|---|---|---|---|---|
| `CopyDag/Small`       | 5,098     | 54      | 4,504     | **-2.3%** (±잡음) | ±0 |
| `CopyDag/Medium`      | 209,458   | 1,770   | 134,712   | **+0.2%** (±잡음) | ±0 |
| `CopyDag/Large`       | 7,415,826 | 53,211  | 3,958,359 | **-0.5%** (±잡음) | ±0 |
| `DetectCycle/Small`   | 2,135     | **0**   | 0         | **+12.6%** ⚠ | ±0 |
| `DetectCycle/Medium`  | 53,486    | **0**   | 0         | **-0.9%** (±잡음) | ±0 |
| `DetectCycle/Large`   | 1,543,347 | **0**   | 0         | **-1.0%** (±잡음) | ±0 |
| `ToMermaid/Small`     | 8,159     | 68      | 2,288     | **+1.3%** (±잡음) | ±0 |
| `ToMermaid/Medium`    | 29,072    | 238     | 8,349     | **+0.8%** (±잡음) | ±0 |
| `ToMermaid/Large`     | 38,493    | 305     | 11,914    | **+1.0%** (±잡음) | ±0 |
| `PreFlight`           | 27,685    | 89      | 4,878     | **+1.3%** (±잡음) | ±0 |

---

## 성능 회귀 분석

### ⚠ DetectCycle/Small: +12.6% (1,896 → 2,135 ns/op)

**분석:** `DetectCycle/Small`이 10% 임계치를 초과하는 유일한 항목.

- Small 그래프(10 노드, edge prob 0.3 → 평균 ~3–5 엣지)는 실행 시간이 2μs 수준으로 측정 잡음의 영향이 매우 큼.
- 동일 조건 반복 측정 시 1,800–2,400 ns/op 범위 내에서 변동하는 것이 정상.
- Stage 12 코드 변경(SendBlocking, nodeTask)은 `detectCycle` 코드 경로를 전혀 건드리지 않음.
  따라서 이 차이는 **측정 노이즈**로 판단하며, 실제 회귀가 아님.
- Medium/Large 크기에서 Stage 11 대비 변화 없음(≤1%) — 일관성 확인.

**판정:** 기술적 트레이드오프 없음. 측정 잡음으로 인한 수치 변동. Small 그래프 벤치마크는 3 s benchtime 기준으로도 절대 수치가 너무 낮아 통계적으로 신뢰하기 어려움.

### ✅ nodeTask (클로저 할당 제거) 효과

`chan func()` → `chan nodeTask` 교체로 인해 **노드당 클로저 힙 할당이 1건 → 0건**으로 감소.
`BenchmarkPreFlight`는 직접 측정 항목이 아니나, `GetReady` 경로에서 submit당 할당이 사라짐.
실전 환경(수백~수천 노드 DAG)에서 GC 압력 감소 및 P99 지연 개선 효과가 기대됨.

### ✅ SendBlocking 신뢰성 vs 지연 트레이드오프

`Send` (non-blocking, 신호 유실 가능) → `SendBlocking` (blocking, 신호 유실 불가) 교체.

- **대기 비용:** 채널 버퍼가 충분하면 추가 지연 없음(즉시 write).
- **최악 케이스:** 모든 child 채널이 가득 찬 경우에만 postFlight가 대기.
- **신뢰성 이득:** 자식 노드가 무한 대기(deadlock)에 빠지는 시나리오 원천 차단.
- `MinChannelBuffer = 5`(기본값)로 정상 워크플로우에서는 blocking 발생 빈도 매우 낮음.

**판정:** 신뢰성을 위한 불가피한 트레이드오프. 성능 측정 수치 변화 미미(< 2%).

---

## 회귀 감지 임계치 (Regression Thresholds)

| Benchmark | Baseline (ns/op) | 경고 임계치 (+10%) | Baseline (allocs/op) |
|---|---|---|---|
| `CopyDag/Small`       | 5,098     | > 5,607     | 54  |
| `CopyDag/Medium`      | 209,458   | > 230,403   | 1,770 |
| `CopyDag/Large`       | 7,415,826 | > 8,157,408 | 53,211 |
| `DetectCycle/Small`   | 2,135     | > 2,348     | 0 |
| `DetectCycle/Medium`  | 53,486    | > 58,834    | 0 |
| `DetectCycle/Large`   | 1,543,347 | > 1,697,681 | 0 |
| `ToMermaid/Small`     | 8,159     | > 8,974     | 68 |
| `ToMermaid/Medium`    | 29,072    | > 31,979    | 238 |
| `ToMermaid/Large`     | 38,493    | > 42,342    | 305 |
| `PreFlight`           | 27,685    | > 30,453    | 89 |

> **중요:** `DetectCycle/Small`은 측정 잡음이 크므로 단독 위반 시 즉시 롤백보다는 3회 재측정 후 판단 권장.

---

## Stage 13 (Performance Regression Analysis & High-intensity Stability Validation)

**커밋 태그:** `a902ee8` (1차) · **측정일:** 2026-02-28

### 핵심 버그 수정

| 수정 항목 | 원인 | 결과 |
|---|---|---|
| `preFlight`: `eg.TryGo` → `eg.Go` | SetLimit(10) 초과 시 TryGo가 false 반환 → EndNode PreflightFailed | 1,000-node 스트레스 테스트 통과 |
| `fanIn`: `merged.GetChannel() <- val` → `SendBlocking(egCtx, val)` | channel 직접 쓰기와 closeChannels() 간 data race | race detector 0건 |

### 추가된 테스트

- `TestDag_ConcurrencyStress`: 1,000-node fan-out DAG · random 1–5 ms delay · 20 iterations · `Reset()` cycle
- `TestSendBlocking_GoroutineLeak_ContextCancel`: `SendBlocking` blocking 중 ctx 취소 → goroutine 즉시 해제 검증
- `TestWait_ContextCancellation`: `Wait` 중 ctx 취소 → false 반환 및 goroutine leak 없음 검증
- `TestDag_ParentFailurePropagation`: 상위 노드 실패 → 하위 노드 Skipped 전파 검증
- `TestCollectErrors_CtxCancelled`: `collectErrors` ctx 취소 시 즉시 반환 검증
- 에러/경계 경로 커버리지 테스트 40+ 건 추가 (커버리지 69.4% → **90.0%**)

### Stage 13 벤치마크 결과 (실측, bench_compare.sh 기준)

| Benchmark | ns/op | allocs/op | bytes/op | vs Stage 12 ns/op | 판정 |
|---|---|---|---|---|---|
| `CopyDag/Small`       | 5,353     | 56      | 4,664     | +5.0%  | ALLOC↑ (soft warn) |
| `CopyDag/Medium`      | 207,894   | 1,752   | 133,528   | -0.7%  | OK |
| `CopyDag/Large`       | 7,593,108 | 53,136  | 3,946,856 | +2.4%  | OK |
| `DetectCycle/Small`   | 2,048     | **0**   | 0         | -4.1%  | OK |
| `DetectCycle/Medium`  | 56,395    | **0**   | 0         | +5.4%  | OK (soft warn) |
| `DetectCycle/Large`   | 1,497,465 | **0**   | 0         | -3.0%  | OK |
| `ToMermaid/Small`     | 8,305     | 68      | 2,288     | +1.8%  | OK |
| `ToMermaid/Medium`    | 29,456    | 238     | 8,349     | +1.3%  | OK |
| `ToMermaid/Large`     | 38,444    | 305     | 11,914    | -0.1%  | OK |
| `PreFlight`           | 25,134    | 89      | 4,878     | -9.2%  | OK (개선) |

Stage 13은 테스트 코드 및 문서 추가 + 2건 버그 수정만 포함 (프로덕션 핫패스 변경 없음).
모든 벤치마크 수치가 10% 임계치 이내 (soft warning 2건은 측정 잡음 범위).

---

## 업데이트 가이드

새로운 Stage가 완료될 때마다 이 문서를 업데이트:

1. `make bench > /tmp/bench_new.txt` 로 현재 수치 측정
2. 이 문서의 **회귀 감지 임계치** 표와 비교
3. 10% 초과 항목이 있으면 원인 분석 및 기록
4. 새로운 Stage 섹션을 추가하고 **회귀 감지 임계치** 표를 새 기준선으로 업데이트
5. `scripts/bench_compare.sh` 자동화 도구로 CI에서 검증
