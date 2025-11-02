package dag_go

import (
	"context"
	"fmt"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/seoyhaein/dag-go/debugonly"
	"golang.org/x/sync/errgroup"
)

// NodeStatus 는 노드의 현재 상태를 나타냄.
type NodeStatus int

const (
	NodeStatusPending NodeStatus = iota
	NodeStatusRunning
	NodeStatusSucceeded
	NodeStatusFailed
	NodeStatusSkipped // 부모가 실패했을 경우
)

// Node DAG 의 기본 구성 요소
type Node struct {
	ID        string
	ImageName string

	// deprecated: RunCommand 는 더 이상 사용되지 않음.
	RunCommand Runnable

	// 추가 : 안전한 동시성 처리: 러너 스냅샷을 위한 atomic.Value
	// (지우지 말것 ) atomic.Value 는 개발시 주의가 필요함. 해당 내용을 잘 이해하고 있어야함.
	// 이건 항상 immutable 한 값을 저장하는 용도로만 사용해야 함.
	// 포인터를 저장하는 용도로만 사용하고, 값 타입은 피할 것.
	// 반드시 첫 Store는 non-nil : n.runnerVal.Store(&runnerSlot{})처럼 포인터 래퍼 자체는 non-nil이면 OK. (래퍼 내부의 인터페이스 r는 nil이어도 됨)
	// 복사 금지: atomic.Value는 “첫 사용 이후엔 복사 금지”가 원칙이라, Node를 값 복사하는 패턴은 피하세요(포인터로 다루기).
	// 동시성: Load/Store는 다중 고루틴에서 동시에 호출해도 안전, CAS 에 관해서는 별도의 노션으로 정리하자.일단 readme 에 남겨둠.

	runnerVal atomic.Value // stores *runnerSlot

	children  []*Node // 자식 노드 리스트
	parent    []*Node // 부모 노드 리스트
	parentDag *Dag    // 자신이 속한 DAG
	Commands  string

	// childrenVertex 와 parentVertex 를 SafeChannel 슬라이스로 관리
	childrenVertex []*SafeChannel[runningStatus]
	parentVertex   []*SafeChannel[runningStatus]
	runner         func(ctx context.Context, result *SafeChannel[*printStatus])

	// 동기화를 위한 필드
	status  NodeStatus
	succeed bool
	mu      sync.RWMutex // 공유 상태 보호를 위한 뮤텍스

	// 타임아웃 관련 설정 (각 노드별 설정)
	Timeout  time.Duration // 타임아웃 시간 (예: 5초 등)
	bTimeout bool          // true 이면 타임아웃 적용, false 면 무한정 기다림
}

// SetStatus 노드의 상태를 안전하게 설정
func (n *Node) SetStatus(status NodeStatus) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.status = status
}

// GetStatus 노드의 상태를 안전하게 반환함
func (n *Node) GetStatus() NodeStatus {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.status
}

// SetSucceed 노드의 succeed 필드를 안전하게 설정함
func (n *Node) SetSucceed(val bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.succeed = val
}

// IsSucceed 노드의 succeed 필드를 안전하게 반환함
func (n *Node) IsSucceed() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.succeed
}

// CheckParentsStatus 부모 노드의 상태를 확인함
// 부모 중 하나라도 실패하면 false 를 반환함
func (n *Node) CheckParentsStatus() bool {
	for _, parent := range n.parent {
		if parent.GetStatus() == NodeStatusFailed {
			// 부모가 실패하면 이 노드를 건너뜁니다.
			n.SetStatus(NodeStatusSkipped)
			return false
		}
	}
	return true
}

// MarkCompleted 노드가 완료되었음을 표시하고 부모 DAG 에 알림
func (n *Node) MarkCompleted() {
	if n.parentDag != nil && n.ID != StartNode && n.ID != EndNode {
		atomic.AddInt64(&n.parentDag.completedCount, 1)
	}
}

// NodeError 노드 실행 중 발생한 오류를 나타냄
type NodeError struct {
	NodeID string
	Phase  string
	Err    error
}

func (e *NodeError) Error() string {
	return fmt.Sprintf("node %s failed in %s phase: %v", e.NodeID, e.Phase, e.Err)
}

func (e *NodeError) Unwrap() error {
	return e.Err
}

// preFlight 노드의 실행 전 단계를 처리함
// TODO preFlight의 30초 하드코딩 타임아웃 이거 개선해야 함.
func preFlight(ctx context.Context, n *Node) *printStatus {
	if n == nil {
		return newPrintStatus(PreflightFailed, noNodeID)
	}

	// TODO 실제 분석 파이프라인에서 테스트 해봐야 함. 추후 수정 필요.
	//nolint:mnd // 부모 컨텍스트에서 타임아웃 설정 30초
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// errgroup 및 새로운 컨텍스트 생성
	eg, egCtx := errgroup.WithContext(timeoutCtx)
	//nolint:mnd // 10 is the fixed limit for concurrent goroutines. //TODO 향후 수정.
	eg.SetLimit(10) // 동시 실행 고루틴 수 제한

	i := len(n.parentVertex)
	var try = true

	for j := 0; j < i; j++ { //nolint:intrange
		k := j // closure 캡처 문제 해결
		// SafeChannel 포인터를 가져옴. nil 체크 수행.
		sc := n.parentVertex[k]
		if sc == nil {
			Log.Fatalf("preFlight: n.parentVertex[%d] is nil for node %s", k, n.ID)
		}
		try = eg.TryGo(func() error {
			nodeID, chIdx := n.ID, k
			// 라벨을 먼저 걸고
			lbl := pprof.Labels(
				"phase", "preFlight",
				"nodeId", nodeID,
				"channelIndex", strconv.Itoa(chIdx),
			)

			pprof.SetGoroutineLabels(pprof.WithLabels(egCtx, lbl)) // 현재 고루틴에 라벨 즉시 적용

			// 원하는 지점에서 중단
			if nodeID == "C" {
				debugonly.BreakHere() // // cli 실행 시점에서는 이게 멈춤.
			}

			if nodeID == "node1" && chIdx == 2 {
				debugonly.BreakHere() // 다른 코드에서 멈춤.
			}

			// 디버깅 라벨 설정
			/*			debugger.SetLabels(func() []string {
						return []string{
							"preFlight: nodeId", n.ID,
							"channelIndex", strconv.Itoa(k),
						}
					})*/
			select {
			// 내부 채널을 통해 값을 읽어온다.
			case result := <-sc.GetChannel():
				if result == Failed {
					return fmt.Errorf("node %s: parent channel returned Failed", n.ID)
				}
				return nil
			case <-egCtx.Done():
				return egCtx.Err()
			}
		})
		if !try {
			break
		}
	}

	err := eg.Wait()
	if err == nil && try {
		n.SetSucceed(true)
		Log.Println("Preflight", n.ID)
		return newPrintStatus(Preflight, n.ID)
	}

	// 에러 발생 시 구조화된 에러 생성 및 로깅
	if err != nil {
		nodeErr := &NodeError{
			NodeID: n.ID,
			Phase:  "preflight",
			Err:    err,
		}
		Log.Println(nodeErr.Error())
	}

	n.SetSucceed(false)
	Log.Println("Preflight failed for node", n.ID, "error:", err)
	return newPrintStatus(PreflightFailed, n.ID)
}

// inFlight 노드의 실행 단계를 처리
// TODO context 적용해줘야 함.
func inFlight(n *Node) *printStatus {
	if n == nil {
		return newPrintStatus(InFlightFailed, noNodeID)
	}

	if n.ID == StartNode || n.ID == EndNode {
		n.SetSucceed(true)
		Log.Println("InFlight (special node)", n.ID)
		return newPrintStatus(InFlight, n.ID)
	}

	// 일반 노드의 경우
	if n.IsSucceed() {
		n.SetStatus(NodeStatusRunning)
		if err := n.Execute(); err != nil {
			n.SetSucceed(false)
			nodeErr := &NodeError{
				NodeID: n.ID,
				Phase:  "inflight",
				Err:    err,
			}
			Log.Println(nodeErr.Error())
		}
	} else {
		Log.Println("Skipping execution for node", n.ID, "due to previous failure")
	}

	// 최종 결과 판단: 일반 노드의 경우에만 succeed 값을 비교
	if n.IsSucceed() {
		Log.Println("InFlight", n.ID)
		return newPrintStatus(InFlight, n.ID)
	}
	Log.Println("InFlightFailed", n.ID)
	return newPrintStatus(InFlightFailed, n.ID)
}

// postFlight 노드의 실행 후 단계를 처리함
func postFlight(n *Node) *printStatus {
	if n == nil {
		return newPrintStatus(PostFlightFailed, noNodeID)
	}

	// 종료 노드(EndNode)인 경우 별도로 처리
	if n.ID == EndNode {
		Log.Println("FlightEnd", n.ID)
		return newPrintStatus(FlightEnd, n.ID)
	}

	// n.succeed 값에 따라 결과 결정
	var result runningStatus
	if n.IsSucceed() {
		result = Succeed
	} else {
		result = Failed
	}

	// 모든 자식 채널(안전 채널)에 result 값을 보냄.
	// SafeChannel 의 Send 메서드를 이용해 값 전송.
	for _, sc := range n.childrenVertex {
		sc.Send(result)
		// 채널 닫기는 DAG 의 closeChannels() 등에서 관리함.
	}

	// 노드 완료 표시
	n.MarkCompleted()

	Log.Println("PostFlight", n.ID)
	return newPrintStatus(PostFlight, n.ID)
}

// createNode 새로운 노드를 생성
func createNode(id string, r Runnable) *Node {
	n := &Node{ID: id, status: NodeStatusPending}
	n.runnerStore(r) // 첫 Store (non-nil 포인터 래퍼)
	return n
}

// createNodeWithID ID 만으로 새로운 노드를 생성
func createNodeWithID(id string) *Node {
	n := &Node{ID: id, status: NodeStatusPending}
	n.runnerStore(nil) // 첫 Store (nil 러너라도 래퍼는 non-nil)
	return n
}

// TODO 생각해보기 timeout 은 여기 들어가야 하는게 맞을듯.

// Execute 노드의 실행 로직을 구현
func (n *Node) Execute() error {
	return execute(n)
}

// execute RunCommand 실행
func execute(this *Node) error {
	r := this.getRunnerSnapshot() // 실행 직전 러너 스냅샷
	if r == nil {
		return ErrNoRunner // 명시적 실패 가드
	}
	return r.RunE(this)
}

func (n *Node) notifyChildren(st runningStatus) {
	for _, sc := range n.childrenVertex {
		_ = sc.Send(st)
	}
}

// checkVisit 모든 노드가 방문되었는지 확인함
//
//nolint:unused // This function is intentionally left for future use.
func checkVisit(visit map[string]bool) bool {
	for _, v := range visit {
		if !v {
			return false
		}
	}
	return true
}

// getNode 노드 맵에서 지정된 ID의 노드를 반환함
//
//nolint:unused // This function is intentionally left for future use.
func getNode(s string, ns map[string]*Node) *Node {
	if strings.TrimSpace(s) == "" {
		return nil
	}

	if len(ns) == 0 {
		return nil
	}

	return ns[s]
}

// getNextNode 첫 번째 자식 노드를 가져오고 해당 노드를 삭제함
//
//nolint:unused // This function is intentionally left for future use.
func getNextNode(n *Node) *Node {
	if n == nil {
		return nil
	}
	if len(n.children) < 1 {
		return nil
	}

	ch := n.children[0]
	n.children = append(n.children[:0], n.children[1:]...)
	return ch
}

// statusPool printStatus 객체 풀
var statusPool = sync.Pool{
	New: func() interface{} {
		return &printStatus{}
	},
}

// newPrintStatus printStatus 객체를 생성
func newPrintStatus(status runningStatus, nodeID string) *printStatus {
	ps := statusPool.Get().(*printStatus)
	ps.rStatus = status
	ps.nodeID = nodeID
	return ps
}

// releasePrintStatus printStatus 객체를 풀에 반환
func releasePrintStatus(ps *printStatus) {
	ps.rStatus = 0
	ps.nodeID = ""
	statusPool.Put(ps)
}
