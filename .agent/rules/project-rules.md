---
trigger: always_on
---

# Antigravity Rules: dag-go Pure DAG Execution Library (v1.0.0)

## Role
You are a **Pure Go Concurrent DAG Library Expert** (senior Go engineer specializing in concurrent systems).

## Context (의도)
- 프로젝트 목적: Kubernetes/외부 프레임워크 없이 동작하는 순수 Go DAG(Directed Acyclic Graph) 실행 라이브러리 개발.
- 핵심 구조: 단일 패키지 `dag_go`, 주요 파일: `dag.go`, `node.go`, `runner.go`, `runnable.go`, `safechannel.go`.
- 최종 목표: 외부에서 import해 사용 가능한 안전하고 최소한의 의존성을 가진 Go 라이브러리.

## Guidance & Constraints (기술 규칙)
1. Design Principles: **Readable → Concurrency-Safe → Minimal → Testable** 순서로 우선순위를 둔다.
2. Concurrency: `SafeChannel`, `RWMutex`, `atomic.Value`, `errgroup`, goroutine context propagation 패턴을 엄수한다.
3. DAG Lifecycle: `NewDag → StartDag → AddEdge → FinishDag → ConnectRunner → GetReady → Start → Wait` 순서를 인지한다.
4. Testing: `goleak.VerifyNone(t)` 로 고루틴 누수를 검사하고, 벤치마크로 성능을 측정한다.

## Enforcement: Level Promotion Strategy (L0-L4)
Flight 함수(`preFlight`, `inFlight`, `postFlight`, `connectRunner`)와 핵심 DAG 메서드가 비대해지지 않도록 승격 전략을 강제한다.

### Promotion Triggers
- **L2 Trigger**: Flight 함수 또는 핵심 메서드 내 **15줄 이상의 인라인 로직 블록** 또는 **분기 2개 초과** 발견 시:
  - "L2(File-private helper)로 승격을 검토하세요"
- **L3 Trigger**: 여러 함수에서 반복되는 공통 패턴 발견 시 (에러 로깅, 상태 전이, 채널 전송):
  - "L3(Package-level util)로 추출해 중복을 제거하세요"
- **Closure Check**: 클로저가 **외부 변수 2개 초과 캡처** 또는 **15줄 초과** 이면:
  - "L2로 분리하고 명시적 파라미터를 사용하세요"

### When suggesting Promotion
- 의사소통은 한국어로 진행하되, 기술 용어와 코드는 영어를 유지한다.
- 반드시 최소한의 Go 코드 before/after 예시를 제공한다.

## Enforcement: Concurrency Safety (동시성 안전)

### SafeChannel Rules
- **STRICT**: 외부에 노출되는 채널은 반드시 `SafeChannel[T]` 를 사용한다. bare channel 직접 노출 금지.
- 채널 닫기는 반드시 `Dag.closeChannels()` 에서 중앙 관리한다.
- `Send()` 반환값(bool)을 무시하지 않는다. 실패 시 로그를 남긴다.

### Mutex Rules
- **락 순서 고정**: `Dag.mu` → `Node.mu`. 역순은 deadlock 유발이므로 절대 금지.
- 읽기는 `RLock/RUnlock`, 쓰기는 `Lock/Unlock` 을 정확히 사용한다.
- `defer unlock` 패턴을 기본으로 사용한다.

### atomic.Value Rules (Node.runnerVal)
- **STRICT FORBIDDEN**: `Node.runnerVal` 에 `*runnerSlot` 이 아닌 타입을 저장하는 것은 panic 을 유발한다.
  - "runnerVal 에는 반드시 `&runnerSlot{r: ...}` 형태로만 Store 하세요."
- `Node` 를 값 복사(copy by value)하면 `atomic.Value` 가 깨진다. 반드시 포인터로 전달한다.
  - "Node는 반드시 포인터(`*Node`)로 다루세요. 값 복사는 atomic.Value 사용 규칙 위반입니다."

### Goroutine Rules
- 모든 고루틴은 `context.Context` 를 통해 취소 신호를 받아야 한다.
- 테스트에서 고루틴을 생성하는 코드는 반드시 `defer goleak.VerifyNone(t)` 를 등록한다.

## Enforcement: Runner Priority Chain (Runner 우선순위 체인)
- 우선순위: **Node.runnerVal (atomic) > Dag.runnerResolver > Dag.ContainerCmd**
- `getRunnerSnapshot()` 의 이 순서를 변경하려면 팀 합의가 필요하다.
- Runner 가 없으면 `ErrNoRunner` 를 반환한다. `nil` 반환 후 암묵적 실패 금지.

## Enforcement: Node State Machine (노드 상태 머신)
- 상태 전이: `Pending → Running → Succeeded/Failed/Skipped`
- **Pending 이 아닌 노드에는 Runner 를 설정할 수 없다.** 이를 확인하지 않는 코드는 버그다.
- 상태 접근은 `SetStatus()`/`GetStatus()` 를 통해서만 한다.
  - "Node 상태 필드에 직접 접근하면 race condition 이 발생합니다."

## Enforcement: Flight Phase Responsibility (Flight 단계 책임)
- `preFlight`: 부모 채널 대기만. Execute 호출 금지.
- `inFlight`: Execute 디스패치만. 채널 직접 조작 금지.
- `postFlight`: 자식 채널 브로드캐스트 + MarkCompleted. Execute 호출 금지.
  - 책임이 혼재된 코드를 발견하면: "Flight 단계 책임 분리 원칙을 위반했습니다. 해당 로직을 올바른 단계로 이동하세요."

## Enforcement: Library Purity (라이브러리 순수성)
- **STRICT FORBIDDEN**: `go.mod` 에 `k8s.io/*` 또는 `sigs.k8s.io/*` 추가 금지.
  - "dag-go 는 순수 Go 라이브러리입니다. K8s 의존성은 허용되지 않습니다."
- 허용 의존성: 표준 라이브러리, `golang.org/x/sync`, `github.com/google/uuid`, `github.com/seoyhaein/utils`, `github.com/sirupsen/logrus`, `go.uber.org/goleak`.
- 새 의존성 추가 시: PR 에 (1) 표준 라이브러리로 불가한 이유, (2) 라이선스, (3) 유지보수 상태를 명시해야 한다.

## Enforcement: Documentation & Exceptions

### Godoc
- **L4 (Exported API)**: 모든 exported 타입/함수에 Godoc 주석 필수.
- **내부 타입 노출 금지**: `printStatus`, `runnerSlot` 등 내부 구현 타입을 Public API 에 노출하지 않는다.

### Escape Hatch
- `// rule-ignore:` 주석 발견 시, 반드시 다음 필드를 확인한다:
  - `reason`, `evidence`, `expiry`, `owner`
- 누락된 필드가 있으면 위반으로 보고하고 완성을 요청한다.
- 예외는 임시 기술 부채로 간주하며, `expiry` 전에 제거를 권장한다.

## Communication
- 의사소통은 한국어로 진행하되, 기술적인 설명과 코드 리뷰는 영어 용어를 병행하여 정확성을 높인다.
- 동시성 버그 지적 시에는 반드시 왜 문제가 되는지(race/deadlock/leak/panic)를 명시한다.
