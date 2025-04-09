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

// NodeStatus는 노드의 현재 상태를 나타냅니다.
type NodeStatus int

const (
	NodeStatusPending NodeStatus = iota
	NodeStatusRunning
	NodeStatusSucceeded
	NodeStatusFailed
	NodeStatusSkipped
)

// Node는 DAG의 기본 구성 요소입니다.
type Node struct {
	Id         string
	ImageName  string
	RunCommand Runnable

	children  []*Node // children
	parent    []*Node // parents
	parentDag *Dag    // 자신이 소속되어 있는 Dag
	Commands  string

	childrenVertex []chan runningStatus
	parentVertex   []chan runningStatus
	runner         func(ctx context.Context, result chan *printStatus)

	// 동기화를 위한 필드 추가
	status  NodeStatus
	succeed bool
	mu      sync.RWMutex // 공유 상태 보호를 위한 뮤텍스
}

// SetStatus는 노드의 상태를 안전하게 설정합니다.
func (n *Node) SetStatus(status NodeStatus) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.status = status
}

// GetStatus는 노드의 상태를 안전하게 반환합니다.
func (n *Node) GetStatus() NodeStatus {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.status
}

// SetSucceed는 노드의 succeed 필드를 안전하게 설정합니다.
func (n *Node) SetSucceed(val bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.succeed = val
}

// IsSucceed는 노드의 succeed 필드를 안전하게 반환합니다.
func (n *Node) IsSucceed() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.succeed
}

// CheckParentsStatus는 부모 노드의 상태를 확인합니다.
// 부모 중 하나라도 실패했으면 false를 반환합니다.
func (n *Node) CheckParentsStatus() bool {
	for _, parent := range n.parent {
		if parent.GetStatus() == NodeStatusFailed {
			// 부모가 실패하면 이 노드를 건너뜀
			n.SetStatus(NodeStatusSkipped)
			return false
		}
	}
	return true
}

// MarkCompleted는 노드가 완료되었음을 표시하고 부모 DAG에 알립니다.
func (n *Node) MarkCompleted() {
	if n.parentDag != nil {
		atomic.AddInt64(&n.parentDag.completedCount, 1)
	}
}

// NodeError는 노드 실행 중 발생한 오류를 나타냅니다.
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

// preFlight는 노드의 실행 전 단계를 처리합니다.
// 이 함수는 모든 부모 노드의 상태를 확인하고, 모든 부모가 성공적으로 완료되었는지 확인합니다.
// 컨텍스트 취소 시 적절히 종료됩니다.
//
// 매개변수:
// - ctx: 컨텍스트, 취소 신호를 전파하는 데 사용됩니다.
// - n: 처리할 노드
//
// 반환값:
// - *printStatus: 노드의 실행 상태
func preFlight(ctx context.Context, n *Node) *printStatus {
	if n == nil {
		return newPrintStatus(PreflightFailed, noNodeId)
	}

	// 부모 컨텍스트에서 타임아웃 설정
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// errgroup 과 새로운 컨텍스트 생성
	eg, egCtx := errgroup.WithContext(timeoutCtx)

	// Go 1.18 이상에서 사용 가능
	// 동시 실행 고루틴 수를 제한
	eg.SetLimit(10)

	i := len(n.parentVertex)
	var try bool = true

	for j := 0; j < i; j++ {
		k := j // closure 캡처 문제 해결
		c := n.parentVertex[k]
		try = eg.TryGo(func() error {
			// 각 고루틴에 디버깅 라벨 설정
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
		// do not erase
		Log.Println("Preflight", n.Id)
		return newPrintStatus(Preflight, n.Id)
	}

	// 에러 발생 시 구조화된 에러 생성
	if err != nil {
		nodeErr := &NodeError{
			NodeID: n.Id,
			Phase:  "preflight",
			Err:    err,
		}
		Log.Println(nodeErr.Error())
	}

	n.SetSucceed(false)
	// do not erase
	Log.Println("Preflight failed for node", n.Id, "error:", err)
	return newPrintStatus(PreflightFailed, noNodeId)
}

// inFlight는 노드의 실행 단계를 처리합니다.
// preFlight, inFlight, postFlight 에서의 node 는 같은 노드이다.
// runner 에서 순차적으로 동작한다.
func inFlight(n *Node) *printStatus {
	if n == nil {
		return newPrintStatus(InFlightFailed, noNodeId)
	}

	// 시작 노드와 종료 노드는 실행 없이 succeed 를 true 로 설정
	if n.Id == StartNode || n.Id == EndNode {
		n.SetSucceed(true)
	} else if n.IsSucceed() { // IsSucceed() 메서드 사용
		if err := n.Execute(); err != nil {
			n.SetSucceed(false)
			// 에러 발생 시 구조화된 에러 생성
			nodeErr := &NodeError{
				NodeID: n.Id,
				Phase:  "inflight",
				Err:    err,
			}
			Log.Println(nodeErr.Error())
		}
	}

	// 결과에 따라 Log 를 통해 메시지를 기록하고 printStatus 를 반환
	if n.IsSucceed() {
		Log.Println("InFlight", n.Id)
		return newPrintStatus(InFlight, n.Id)
	}

	Log.Println("InFlightFailed", n.Id)
	return newPrintStatus(InFlightFailed, n.Id)
}

// postFlight는 노드의 실행 후 단계를 처리합니다.
// preFlight, inFlight, postFlight 에서의 node 는 같은 노드이다.
// runner 에서 순차적으로 동작한다.
func postFlight(n *Node) *printStatus {
	if n == nil {
		return newPrintStatus(PostFlightFailed, noNodeId)
	}

	// 종료 노드(EndNode)인 경우 별도로 처리
	if n.Id == EndNode {
		Log.Println("FlightEnd", n.Id)
		return newPrintStatus(FlightEnd, n.Id)
	}

	// n.succeed 값에 따라 결과를 결정
	var result runningStatus
	if n.IsSucceed() {
		result = Succeed
	} else {
		result = Failed
	}

	// 모든 자식 채널에 result 값을 보내고, 채널을 닫지 않음
	// 채널 닫기는 DAG 레벨에서 관리
	for _, c := range n.childrenVertex {
		select {
		case c <- result:
			// 성공적으로 전송됨
		case <-time.After(5 * time.Second):
			Log.Printf("Warning: Timeout sending result to channel for node %s", n.Id)
		default:
			// 채널이 이미 닫혔거나 가득 찬 경우 처리
			Log.Printf("Warning: Could not send to channel for node %s", n.Id)
		}
	}

	// 노드 완료 표시
	n.MarkCompleted()

	Log.Println("PostFlight", n.Id)
	return newPrintStatus(PostFlight, n.Id)
}

// createNode는 새로운 노드를 생성합니다.
func createNode(id string, r Runnable) *Node {
	return &Node{
		Id:         id,
		RunCommand: r,
		status:     NodeStatusPending,
	}
}

// createNodeWithId는 ID만으로 새로운 노드를 생성합니다.
func createNodeWithId(id string) *Node {
	return &Node{
		Id:     id,
		status: NodeStatusPending,
	}
}

// Execute는 노드의 실행 로직을 구현합니다.
func (n *Node) Execute() (err error) {
	if n.RunCommand != nil {
		err = execute(n)
		return
	}
	// Container 를 사용하 지않는 다른 명령어를 넣을 경우 여기서 작성하면 된다.
	return nil
}

// execute는 RunCommand를 실행합니다.
func execute(this *Node) error {
	err := this.RunCommand.RunE(this)
	return err
}

// checkVisit는 모든 노드가 방문되었는지 확인합니다.
func checkVisit(visit map[string]bool) bool {
	for _, v := range visit {
		if v == false {
			return false
		}
	}
	return true
}

// getNode는 노드 맵에서 지정된 ID의 노드를 반환합니다.
func getNode(s string, ns map[string]*Node) *Node {
	if len(strings.TrimSpace(s)) == 0 {
		return nil
	}

	size := len(ns)
	if size <= 0 {
		return nil
	}

	n := ns[s]
	return n
}

// getNextNode는 첫 번째 자식 노드를 가져오고 해당 노드를 삭제합니다.
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

// newPrintStatus는 printStatus 객체를 생성합니다.
func newPrintStatus(status runningStatus, nodeId string) *printStatus {
	ps := statusPool.Get().(*printStatus)
	ps.rStatus = status
	ps.nodeId = nodeId
	return ps
}

// releasePrintStatus는 printStatus 객체를 풀에 반환합니다.
func releasePrintStatus(ps *printStatus) {
	// 필드 초기화
	ps.rStatus = 0
	ps.nodeId = ""
	statusPool.Put(ps)
}
