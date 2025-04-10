package dag_go

import (
	"context"
	"fmt"
	"golang.org/x/sync/errgroup"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	// (do not erase) goroutine 디버깅용
	"github.com/dlsniper/debugger"
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
	Id         string
	ImageName  string
	RunCommand Runnable

	children  []*Node // 자식 노드 리스트
	parent    []*Node // 부모 노드 리스트
	parentDag *Dag    // 자신이 소속된 DAG
	Commands  string

	// TODO 추후 수정. Edge 타입으로 채널 수정하는 것으로 해보자.
	childrenVertex []chan runningStatus
	parentVertex   []chan runningStatus
	runner         func(ctx context.Context, result chan *printStatus)

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
	if n.parentDag != nil {
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
func preFlight(ctx context.Context, n *Node) *printStatus {
	if n == nil {
		return newPrintStatus(PreflightFailed, noNodeId)
	}

	// 부모 컨텍스트에서 타임아웃 설정 (30초; 추후 수정 가능)
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// errgroup 새로운 컨텍스트 생성
	eg, egCtx := errgroup.WithContext(timeoutCtx)

	// 동시 실행 고루틴 수 제한 (Go 1.18 이상)
	eg.SetLimit(10)

	i := len(n.parentVertex)
	var try bool = true

	for j := 0; j < i; j++ {
		k := j // closure 캡처 문제 해결
		// 배열 요소가 nil 인지 확인하는 어설션 (방어적 프로그래밍)
		c := n.parentVertex[k]
		if c == nil {
			Log.Fatalf("preFlight: n.parentVertex[%d] is nil for node %s", k, n.Id)
		}
		try = eg.TryGo(func() error {
			// 디버깅 라벨 설정
			debugger.SetLabels(func() []string {
				return []string{
					"preFlight: nodeId", n.Id,
					"channelIndex", strconv.Itoa(k),
				}
			})
			select {
			case result := <-c:
				if result == Failed {
					return fmt.Errorf("node %s: parent channel returned Failed", n.Id)
				}
				return nil
			case <-egCtx.Done():
				// 컨텍스트 취소 시 에러 반환
				return egCtx.Err()
			}
		})
		if !try {
			break
		}
	}

	// 모든 고루틴이 종료될 때까지 대기
	err := eg.Wait()
	if err == nil && try {
		n.SetSucceed(true)
		Log.Println("Preflight", n.Id)
		return newPrintStatus(Preflight, n.Id)
	}

	// 에러 발생 시 구조화된 에러 생성 및 로깅
	if err != nil {
		nodeErr := &NodeError{
			NodeID: n.Id,
			Phase:  "preflight",
			Err:    err,
		}
		Log.Println(nodeErr.Error())
	}

	n.SetSucceed(false)
	Log.Println("Preflight failed for node", n.Id, "error:", err)
	return newPrintStatus(PreflightFailed, n.Id)
}

// inFlight 노드의 실행 단계를 처리
func inFlight(n *Node) *printStatus {
	if n == nil {
		return newPrintStatus(InFlightFailed, noNodeId)
	}

	if n.Id == StartNode || n.Id == EndNode {
		n.SetSucceed(true)
		Log.Println("InFlight (special node)", n.Id)
		return newPrintStatus(InFlight, n.Id)
	}

	// 일반 노드의 경우
	if n.IsSucceed() {
		n.SetStatus(NodeStatusRunning)
		if err := n.Execute(); err != nil {
			n.SetSucceed(false)
			nodeErr := &NodeError{
				NodeID: n.Id,
				Phase:  "inflight",
				Err:    err,
			}
			Log.Println(nodeErr.Error())
		}
	} else {
		Log.Println("Skipping execution for node", n.Id, "due to previous failure")
	}

	// 최종 결과 판단: 일반 노드의 경우에만 succeed 값을 비교합니다.
	if n.IsSucceed() {
		Log.Println("InFlight", n.Id)
		return newPrintStatus(InFlight, n.Id)
	}
	Log.Println("InFlightFailed", n.Id)
	return newPrintStatus(InFlightFailed, n.Id)
}

// postFlight 노드의 실행 후 단계를 처리함 TODO 예전 코드랑 비교해보자. 흠...
func postFlight(n *Node) *printStatus {
	if n == nil {
		return newPrintStatus(PostFlightFailed, noNodeId)
	}

	// 종료 노드(EndNode)인 경우 별도로 처리
	if n.Id == EndNode {
		Log.Println("FlightEnd", n.Id)
		return newPrintStatus(FlightEnd, n.Id)
	}

	// n.succeed 값에 따라 결과 결정
	var result runningStatus
	if n.IsSucceed() {
		result = Succeed
	} else {
		result = Failed
	}
	// 모든 자식 채널에 result 값을 보냄.
	for _, c := range n.childrenVertex {
		c <- result
		// Important: 이건 dag 의 closeChannels() 에서 채널을 닫아줌. 지우지 말것.
		// close(c)
	}
	// 노드 완료 표시
	n.MarkCompleted()

	Log.Println("PostFlight", n.Id)
	return newPrintStatus(PostFlight, n.Id)
}

// createNode 새로운 노드를 생성
func createNode(id string, r Runnable) *Node {
	return &Node{
		Id:         id,
		RunCommand: r,
		status:     NodeStatusPending,
	}
}

// createNodeWithId는 ID 만으로 새로운 노드를 생성
func createNodeWithId(id string) *Node {
	return &Node{
		Id:     id,
		status: NodeStatusPending,
	}
}

// TODO 생각해보기 timeout 은 여기 들어가야 하는게 맞을듯.

// Execute 노드의 실행 로직을 구현
func (n *Node) Execute() (err error) {
	if n.RunCommand != nil {
		err = execute(n)
		return
	}
	return nil
}

// execute RunCommand 실행
func execute(this *Node) error {
	err := this.RunCommand.RunE(this)
	return err
}

// checkVisit 모든 노드가 방문되었는지 확인함
func checkVisit(visit map[string]bool) bool {
	for _, v := range visit {
		if v == false {
			return false
		}
	}
	return true
}

// getNode 노드 맵에서 지정된 ID의 노드를 반환함
func getNode(s string, ns map[string]*Node) *Node {
	if len(strings.TrimSpace(s)) == 0 {
		return nil
	}

	if len(ns) <= 0 {
		return nil
	}

	return ns[s]
}

// getNextNode 첫 번째 자식 노드를 가져오고 해당 노드를 삭제함
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

// printStatus 객체 풀
var statusPool = sync.Pool{
	New: func() interface{} {
		return &printStatus{}
	},
}

// newPrintStatus printStatus 객체를 생성
func newPrintStatus(status runningStatus, nodeId string) *printStatus {
	ps := statusPool.Get().(*printStatus)
	ps.rStatus = status
	ps.nodeId = nodeId
	return ps
}

// releasePrintStatus printStatus 객체를 풀에 반환
func releasePrintStatus(ps *printStatus) {
	ps.rStatus = 0
	ps.nodeId = ""
	statusPool.Put(ps)
}
