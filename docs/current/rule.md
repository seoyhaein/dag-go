# 팀 코드 작성 규칙 v1.0 (dag-go 전용)
**부제: Pure Go Concurrent Library — Readable, Safe, Minimal**

## 목적
dag-go 는 Kubernetes/외부 프레임워크 없이 동작하는 **순수 Go DAG 실행 라이브러리**다.
우리는 코드를 다음 기준으로 유지한다.

- **Readable**: 읽기 쉽다
- **Concurrency-Safe**: 동시성 버그(race, leak, deadlock)가 없다
- **Minimal**: 의존성이 최소화되어 있다
- **Testable**: 단위/통합 테스트가 쉽다

> 원칙: **Flight 함수는 슬림하게, 동시성 원시 타입은 규칙대로, 의존성은 최소로 유지한다.**

---

## 목차
1. [코드 레벨 승격(Level Promotion) 전략](#1-코드-레벨-승격level-promotion-전략)
2. [동시성 규칙: SafeChannel / Mutex / atomic.Value](#2-동시성-규칙-safechannel--mutex--atomicvalue)
3. [Runner 우선순위 체인 규칙](#3-runner-우선순위-체인-규칙)
4. [Node 상태 머신 규칙](#4-node-상태-머신-규칙)
5. [Flight 단계 책임 분리 규칙](#5-flight-단계-책임-분리-규칙)
6. [라이브러리 순수성(Purity) 규칙](#6-라이브러리-순수성purity-규칙)
7. [성능 정책: 측정 없이는 최적화도 없다](#7-성능-정책-측정-없이는-최적화도-없다)
8. [PR 리뷰 체크리스트](#8-pr-리뷰-체크리스트)
9. [실전 예시: 승격이 필요한 결정적 장면](#9-실전-예시-승격이-필요한-결정적-장면)
10. [AI/CI Enforcement: .antigravity_rules & golangci-lint](#10-aici-enforcement-antigravity_rules--golangci-lint)

---

## 1. 코드 레벨 승격(Level Promotion) 전략

dag-go 의 핵심 함수(`preFlight`, `inFlight`, `postFlight`, `connectRunner` 등)는 비대해지기 쉽다.
우리는 로직의 **중요도/재사용성/복잡도**에 따라 단계를 **승격(Promotion)** 한다.

### 1.1 승격 레벨(L0~L4)

| 레벨 | 명칭 | 위치 | 승격 기준(Trigger) |
|---|---|---|---|
| **L0** | Inline | 함수 내부 직접 작성 | 5줄 이하, 단순 가드/분기 |
| **L1** | Closure | 함수 내부 클로저 | 캡처 변수 2개 이하, 짧고 명확한 목적 |
| **L2** | File-private helper | 같은 파일 소문자 함수 | 15줄↑, 분기 2개↑, 단위 테스트 필요 |
| **L3** | Package-level util | 같은 패키지 소문자 유틸 | 여러 함수에서 공통 사용되는 패턴 |
| **L4** | Exported Public API | 대문자 함수/타입 | 외부에서 사용, 안정적 계약, Godoc 필수 |

### 1.2 L1 클로저 규칙

- **허용**: `logErr`, `copyStatus` 같은 짧은 에러 처리/값 복사 클로저 (캡처 ≤2개)
- **금지**: 15줄이 넘거나, 동시성 로직을 포함하거나, 외부 채널을 직접 다루는 클로저
- **주의**: 고루틴 내부 클로저에서 루프 변수 캡처 시 반드시 로컬 변수에 복사할 것

  ```go
  // Bad: 루프 변수 캡처 문제
  for j := 0; j < i; j++ {
      go func() { use(j) }() // j가 공유됨
  }

  // Good: 로컬 변수에 복사
  for j := 0; j < i; j++ {
      k := j
      go func() { use(k) }()
  }
  ```

### 1.3 L4 Exported API 규칙

- Exported 타입/함수에는 반드시 **Godoc 주석**을 작성한다.
- Public API 에는 절대 내부 구현 타입(`printStatus`, `runnerSlot` 등)을 노출하지 않는다.
- Exported API 변경은 하위 호환성을 고려하거나 버전 노트를 남긴다.

---

## 2. 동시성 규칙: SafeChannel / Mutex / atomic.Value

dag-go 의 모든 동시성 버그는 이 규칙을 지키지 않을 때 발생한다.

### 2.1 SafeChannel 규칙

- 패키지 외부로 노출되는 채널은 **반드시 `SafeChannel[T]`** 를 사용한다.
- 내부 bare channel 은 `SafeChannel` 내부에서만 사용한다.
- **닫기(Close)** 책임은 `Dag.closeChannels()` 에서 중앙 관리한다. 개별 함수에서 임의로 닫지 않는다.
- `Send()` 실패(채널 가득 참 또는 닫힘)는 무시하지 않고 로그를 남긴다.

### 2.2 RWMutex 규칙

- `Dag.mu`: DAG의 `nodes` 맵, `runnerResolver`, `ContainerCmd` 접근에 사용
  - 읽기: `RLock/RUnlock`
  - 쓰기: `Lock/Unlock`
- `Node.mu`: Node의 `status`, `succeed` 필드 접근에 사용
- **락 순서 고정**: 항상 `Dag.mu` 먼저, 그 다음 `Node.mu` (역순 절대 금지 → deadlock)
- `defer unlock` 패턴을 기본으로 사용한다.

### 2.3 atomic.Value 규칙 (Node.runnerVal)

- `Node.runnerVal` 에는 **반드시 `*runnerSlot` 타입만** 저장한다.
  ```go
  // Good
  n.runnerVal.Store(&runnerSlot{r: someRunnable})

  // Bad: 직접 인터페이스 저장 → 타입 불일치 panic
  n.runnerVal.Store(someRunnable)
  ```
- 첫 `Store`는 반드시 non-nil 포인터: `&runnerSlot{}` 또는 `&runnerSlot{r: nil}` 허용
- `Node`를 값 복사(copy by value)하지 않는다. **반드시 포인터로 전달**한다.
- `Load` 후 타입 단언은 `v.(*runnerSlot)` 패턴만 사용한다.

### 2.4 고루틴 관리 규칙

- 모든 고루틴은 **`context.Context`** 를 통해 취소 신호를 받을 수 있어야 한다.
- 고루틴 시작 시 `WaitGroup.Add(1)` 또는 `errgroup` 을 사용하고, 반드시 `Done()` 을 `defer` 로 보장한다.
- 테스트에서는 `goleak.VerifyNone(t)` 를 사용해 고루틴 누수를 검사한다.
- `fanIn`, `GetReady` 등 fan-out 패턴에서 컨텍스트 취소를 항상 처리한다.

---

## 3. Runner 우선순위 체인 규칙

Runner 결정 로직은 **고정된 우선순위 체인**을 따른다. 이 순서를 변경하지 않는다.

```
Node.runnerVal (atomic) > Dag.runnerResolver > Dag.ContainerCmd
```

- **Node 레벨**: `runnerStore(r)` 로 원자적으로 저장된 러너 (가장 높은 우선순위)
- **Resolver**: DAG 전체에 설정된 동적 러너 결정 함수
- **기본값**: DAG 전체 기본 러너 (`ContainerCmd`)

규칙:
- `getRunnerSnapshot()` 은 위 순서대로만 체크한다. 로직을 변경하려면 팀 합의가 필요하다.
- 러너가 하나도 없으면 `ErrNoRunner` 를 반환한다. `nil` 반환 후 암묵적 실패를 허용하지 않는다.

---

## 4. Node 상태 머신 규칙

Node 의 상태는 단방향으로만 전이된다.

```
NodeStatusPending → NodeStatusRunning → NodeStatusSucceeded
                                      → NodeStatusFailed
                ↘ NodeStatusSkipped (부모 실패 시)
```

- **Pending 이 아닌 노드에 Runner 를 설정하지 않는다.** (`SetRunner`, `SetNodeRunner` 내부 검사가 있지만, 호출자도 이를 인지해야 한다.)
- 상태 읽기는 `GetStatus()`, 설정은 `SetStatus()` 를 통해서만 한다. 직접 필드 접근 금지.
- `StartNode`/`EndNode` 는 특수 노드다. 일반 노드와 동일하게 취급하지 않는다.

---

## 5. Flight 단계 책임 분리 규칙

각 Flight 함수는 **단 하나의 책임**만 진다. 이를 섞지 않는다.

| 단계 | 책임 | 금지 사항 |
|---|---|---|
| `preFlight` | 부모 채널 대기 (모든 부모 완료 신호 수신) | Execute 호출, 자식 채널 전송 |
| `inFlight` | `Execute()` 디스패치 (실제 작업 실행) | 채널 직접 조작 |
| `postFlight` | 결과 자식 채널 브로드캐스트 + `MarkCompleted()` | Execute 호출, 부모 채널 읽기 |

- Flight 함수 내부에서 15줄이 넘는 로직이 생기면 **L2로 승격**한다.
- `connectRunner` 내부 클로저가 늘어날 경우 별도 파일-private 함수로 분리한다.
- `preFlight` 의 타임아웃은 하드코딩하지 않는다. `DagConfig.DefaultTimeout` 또는 `Node.Timeout` 을 참조한다.

---

## 6. 라이브러리 순수성(Purity) 규칙

dag-go 는 독립 라이브러리다. 외부 프레임워크 의존을 최소화한다.

### 6.1 허용 의존성

| 종류 | 허용 패키지 |
|---|---|
| Go 표준 라이브러리 | 모두 허용 |
| 동시성 확장 | `golang.org/x/sync` |
| 유틸리티 | `github.com/google/uuid`, `github.com/seoyhaein/utils` |
| 로깅 | `github.com/sirupsen/logrus` |
| 테스트 | `go.uber.org/goleak` |

### 6.2 금지 의존성

- `k8s.io/*`, `sigs.k8s.io/*`: Kubernetes 의존성 절대 금지
- 무거운 웹 프레임워크 (gin, echo 등): 불필요
- ORM, DB 드라이버: 불필요

### 6.3 새 의존성 추가 기준

새 외부 패키지를 추가하려면 PR에 다음을 명시한다:
1. 왜 표준 라이브러리로 해결할 수 없는지
2. 해당 패키지의 라이선스
3. 유지보수 상태 (최근 커밋, 이슈 처리 현황)

### 6.4 Blank Import (`_ "pkg"`)

- **허용 위치**: `main.go` 또는 테스트 파일 (`_test.go`)
- **의무 주석**: 어떤 사이드이펙트(등록/초기화)를 기대하는지 명시
- **금지**: 라이브러리 코어 파일에서 blank import

---

## 7. 성능 정책: 측정 없이는 최적화도 없다

- 기본: **가독성이 성능보다 우선**한다.
- 예외: 벤치마크(`go test -bench`) 또는 프로파일(pprof) 근거가 PR에 포함될 때만 최적화 허용
- 최적화 시 반드시 아래를 기록한다:
  - 왜 필요한지
  - 단순한 대안이 왜 실패했는지
  - 벤치/프로파일 근거 (커맨드/결과 요약)
- `sync.Pool` 사용 (현재 `statusPool`) 은 벤치마크로 효과를 검증한 경우에만 추가한다.
- 매직 넘버(Magic Number)는 이름 있는 상수 또는 `DagConfig` 필드로 추출한다.

---

## 8. PR 리뷰 체크리스트

리뷰어는 다음 질문을 던진다.

- **동시성**: 새 공유 상태에 대한 락/원자적 접근이 있는가?
- **고루틴 누수**: 모든 고루틴이 컨텍스트 취소에 반응하는가? `goleak` 테스트가 통과하는가?
- **SafeChannel**: bare channel 대신 SafeChannel 을 사용했는가?
- **락 순서**: `Dag.mu` → `Node.mu` 순서를 지키는가?
- **Runner 체인**: Runner 우선순위 로직을 건드렸다면 합의가 있었는가?
- **승격 필요성**: Flight 함수 내부 로직이 15줄을 넘기 전에 L2로 분리했는가?
- **순수성**: 새 의존성이 추가되었다면 정당한 근거가 있는가?
- **성능**: 최적화가 있다면 벤치마크 근거가 포함되어 있는가?
- **매직 넘버**: 숫자 리터럴이 상수 또는 Config 필드로 추출되었는가?

---

## 9. 실전 예시: 승격이 필요한 결정적 장면

### Case 1) L1 → L2: 클로저 비대화

**목표**: 고루틴 내부 클로저가 늘어날 때 파일-private 함수로 분리

#### Before (L1: 클로저가 길어지고 로직 혼재)

```go
dag.workerPool.Submit(func() {
    select {
    case <-ctx.Done():
        return
    default:
        // 상태 초기화
        nd.SetStatus(NodeStatusPending)
        // 타임아웃 세팅
        if nd.bTimeout {
            tCtx, cancel := context.WithTimeout(ctx, nd.Timeout)
            defer cancel()
            nd.runner(tCtx, sc)
        } else {
            nd.runner(ctx, sc)
        }
        // 완료 처리
        nd.MarkCompleted()
        // ... 20줄 이상
    }
})
```

#### After (L2: 파일-private 헬퍼로 분리)

```go
dag.workerPool.Submit(func() {
    select {
    case <-ctx.Done():
        return
    default:
        runNodeWithContext(ctx, nd, sc)
    }
})

func runNodeWithContext(ctx context.Context, nd *Node, sc *SafeChannel[*printStatus]) {
    nd.SetStatus(NodeStatusPending)
    execCtx := buildNodeContext(ctx, nd)
    nd.runner(execCtx, sc)
    nd.MarkCompleted()
}
```

---

### Case 2) 동시성 경계: SafeChannel vs bare channel

**목표**: 외부 노출 채널에 SafeChannel 을 사용해 이중 닫기 패닉 방지

#### Before (bare channel 직접 사용)

```go
ch := make(chan runningStatus, 5)
// ... 나중에 누가 닫았는지 추적 불가
close(ch) // 이미 닫혀 있으면 panic
```

#### After (SafeChannel 사용)

```go
sc := NewSafeChannelGen[runningStatus](5)
// ...
if err := sc.Close(); err != nil {
    Log.Warnf("already closed: %v", err)
}
```

---

### Case 3) atomic.Value 잘못된 사용

**목표**: `Node.runnerVal` 에는 항상 `*runnerSlot` 타입만 저장

#### Before (타입 불일치 → panic)

```go
var r Runnable = someRunner
n.runnerVal.Store(r) // 처음 Store 타입이 Runnable이면, 이후 *runnerSlot Store 시 panic
```

#### After (래퍼 타입 고정)

```go
n.runnerVal.Store(&runnerSlot{r: someRunner}) // 항상 *runnerSlot
```

---

## 10. AI/CI Enforcement: .antigravity_rules & golangci-lint

### 10.1 .antigravity_rules (AI 자동 감지 규약)

#### Promotion Detection (승격 감지)

- Flight 함수(`preFlight`, `inFlight`, `postFlight`, `connectRunner`) 내 **연속 15줄 이상의 인라인 로직 블록** 발견 시:
  - "L2(File-private helper)로 승격을 검토하세요"
- 고루틴 내부 클로저가 **외부 변수 캡처 2개 초과** 또는 **15줄 초과** 이면:
  - "L2로 분리하고 시그니처를 명확히 하세요"

#### Concurrency Safety Detection (동시성 안전 감지)

- `Node` 의 `status`, `succeed` 필드에 락 없이 직접 접근 시:
  - "`SetStatus()`/`GetStatus()` 를 통해 접근하세요. 직접 필드 접근은 race condition 을 유발합니다."
- `atomic.Value` 에 `*runnerSlot` 이 아닌 타입을 Store 하는 코드 발견 시:
  - "`runnerVal` 에는 반드시 `*runnerSlot` 타입만 Store 하세요. 타입 불일치 시 panic 이 발생합니다."
- `Dag.mu` 와 `Node.mu` 를 역순으로 획득하는 코드 발견 시:
  - "락 순서는 Dag.mu → Node.mu 로 고정입니다. 역순은 deadlock 을 유발합니다."

#### Goroutine Leak Detection (고루틴 누수 감지)

- 고루틴을 시작하는 함수에서 `context.Context` 를 받지 않으면:
  - "고루틴은 context.Context 를 통해 취소 신호를 처리해야 합니다."
- 테스트에서 `goleak.VerifyNone` 을 사용하지 않으면 (goroutine 을 생성하는 코드 테스트 시):
  - "`goleak.VerifyNone(t)` 를 defer 로 등록해 고루틴 누수를 검사하세요."

#### Library Purity Detection (라이브러리 순수성 감지)

- `go.mod` 에 `k8s.io/*` 또는 `sigs.k8s.io/*` 가 추가되면:
  - "dag-go 는 순수 Go 라이브러리입니다. K8s 의존성은 허용되지 않습니다."
- 허용 목록 외 새 외부 패키지가 추가되면:
  - "새 의존성 추가 시 PR 에 정당성(표준 라이브러리로 불가한 이유, 라이선스, 유지보수 상태)을 명시하세요."

#### Magic Number Audit

- 숫자 리터럴이 함수 내부에 직접 쓰이면:
  - "`DagConfig` 필드 또는 이름 있는 상수로 추출하세요."
  - 예외: `0`, `1` 처럼 의미가 자명한 경우

---

### 10.2 golangci-lint (시스템적으로 규칙을 강제)

권장 조합:

- **govet**: `copylock` 활성화 — `atomic.Value` 포함 Node 값 복사 감지
- **staticcheck**: 미사용 코드, 비효율 패턴 감지
- **cyclop** 또는 **gocognit**: Flight 함수 복잡도 상한 설정 → 승격 유도
- **revive**: 스타일/가독성 규칙 보조
- **errcheck**: 에러 무시 방지
- **gosec**: 동시성/보안 취약점 감지

> 목표: "좋은 습관"이 아니라 "CI에서 자동으로 지켜지는 규칙"이 되게 한다.
