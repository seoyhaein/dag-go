package dag_go

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/seoyhaein/utils"
)

// TODO context 확인해야 함.

// ==================== 상수 정의 ====================

const (
	Create createEdgeErrorType = iota
	Exist
	Fault
)

// The status displayed when running the runner on each node.
const (
	Start runningStatus = iota
	Preflight
	PreflightFailed
	InFlight
	InFlightFailed
	PostFlight
	PostFlightFailed
	FlightEnd
	Failed
	Succeed
)

const (
	StartNode = "start_node"
	EndNode   = "end_node"
)

// It is the node ID when the condition that the node cannot be created.
const noNodeID = "-1"

// ==================== 타입 정의 ====================

// DagConfig DAG 구성 옵션을 정의함
type DagConfig struct {
	MinChannelBuffer int
	MaxChannelBuffer int
	StatusBuffer     int
	WorkerPoolSize   int
	DefaultTimeout   time.Duration
}

// DagOption DAG 옵션 함수 타입
type DagOption func(*Dag)

// DagWorkerPool DAG 워커 풀을 구현
type DagWorkerPool struct {
	workerLimit int
	taskQueue   chan func()
	wg          sync.WaitGroup
}

type (
	runningStatus int

	printStatus struct {
		rStatus runningStatus
		nodeID  string
	}
)

// createEdgeErrorType 0 if created, 1 if exists, 2 if error.
type createEdgeErrorType int

// Dag (Directed Acyclic Graph) is an acyclic graph, not a cyclic graph.
// In other words, there is no cyclic cycle in the DAG algorithm, and it has only one direction.
type Dag struct {
	ID    string
	Edges []*Edge

	nodes     map[string]*Node
	StartNode *Node
	EndNode   *Node
	validated bool

	NodesResult *SafeChannel[*printStatus]
	nodeResult  []*SafeChannel[*printStatus]

	// 에러를 모으는 용도.
	errLogs []*systemError
	Errors  chan error // 에러 채널 추가

	// timeout
	Timeout  time.Duration
	bTimeout bool

	// TODO 확인할 것 -추가
	// 전역 기본 러너(실행 시점 반영용)
	ContainerCmd Runnable

	// Resolver
	runnerResolver RunnerResolver

	// 추가된 필드
	Config         DagConfig
	workerPool     *DagWorkerPool
	nodeCount      int64        // 노드 수를 원자적으로 추적
	completedCount int64        // 완료된 노드 수를 원자적으로 추적
	mu             sync.RWMutex // 맵 접근을 보호하기 위한 뮤텍스
}

// Edge is a channel. It has the same meaning as the connecting line connecting the parent and child nodes.
type Edge struct {
	parentID   string
	childID    string
	safeVertex *SafeChannel[runningStatus] // 안전한 채널 추가.
}

// ==================== DAG 기본 및 옵션 함수 ====================

// DefaultDagConfig 기본 DAG 구성을 반환
func DefaultDagConfig() DagConfig {
	return DagConfig{
		MinChannelBuffer: 5,                // 기본값 증가
		MaxChannelBuffer: 100,              // 기존과 동일
		StatusBuffer:     10,               // 기본값 증가
		WorkerPoolSize:   50,               // 기본 워커 풀 크기
		DefaultTimeout:   30 * time.Second, // 기본 타임아웃
	}
}

// NewDag creates a pointer to the Dag structure with default configuration.
func NewDag() *Dag {
	return NewDagWithConfig(DefaultDagConfig())
}

// NewDagWithConfig creates a pointer to the Dag structure with custom configuration.
func NewDagWithConfig(config DagConfig) *Dag {
	return &Dag{
		nodes:       make(map[string]*Node),
		ID:          uuid.NewString(),
		NodesResult: NewSafeChannelGen[*printStatus](config.MaxChannelBuffer),
		Config:      config,
		Errors:      make(chan error, config.MaxChannelBuffer),
	}
}

// NewDagWithOptions creates a pointer to the Dag structure with options.
func NewDagWithOptions(options ...DagOption) *Dag {
	dag := NewDagWithConfig(DefaultDagConfig())

	// 옵션 적용
	for _, option := range options {
		option(dag)
	}

	return dag
}

// InitDag creates and initializes a new DAG.
func InitDag() (*Dag, error) {
	dag := NewDag()
	if dag == nil {
		return nil, fmt.Errorf("failed to run NewDag")
	}
	return dag.StartDag()
}

// ==================== 전역 러너/리졸버 ====================

// SetContainerCmd sets the container command for the DAG.
func (dag *Dag) SetContainerCmd(r Runnable) {
	// dag.ContainerCmd = r

	// 수정
	dag.mu.Lock()
	dag.ContainerCmd = r
	dag.mu.Unlock()

}

// loadDefaultRunnerAtomic 추가
/*func (dag *Dag) loadDefaultRunnerAtomic() Runnable {
	v := dag.defVal.Load()
	if v == nil {
		return nil
	}
	return v.(*runnerSlot).r
}*/

// SetRunnerResolver  Dag 에 Resolver 보관
func (dag *Dag) SetRunnerResolver(rr RunnerResolver) {
	// dag.runnerResolver = rr

	// 수정
	dag.mu.Lock()
	dag.runnerResolver = rr
	dag.mu.Unlock()
}

// 원자적으로 Resolver 반환
/*func (dag *Dag) loadRunnerResolverAtomic() RunnerResolver {
	// rrVal은 단지 "초기화 여부"를 위한 타입 고정용이고,
	// 실제 rr는 락으로 보호된 dag.runnerResolver에서 읽는다.
	// TODO 완전히 락-프리로 하려면 rrVal에 rr 자체를 담는 별도 래퍼 타입을 써야 함.
	dag.mu.RLock()
	rr := dag.runnerResolver
	dag.mu.RUnlock()
	return rr
}*/

func (dag *Dag) SetNodeRunner(id string, r Runnable) bool {
	dag.mu.RLock()
	n := dag.nodes[id]
	dag.mu.RUnlock()
	if n == nil {
		return false
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	switch n.status {
	case NodeStatusPending:
		// TODO 백워드 호환을 위해서 넣어둠. 향후 삭제하는 방향으로 진행해야 함.
		n.RunCommand = r
		n.runnerStore(r) // atomic.Value에는 *runnerSlot만 Store
		return true

	case NodeStatusRunning, NodeStatusSucceeded, NodeStatusFailed, NodeStatusSkipped:
		// 선택: 이유를 로깅해두면 추적이 쉬움
		Log.Infof("SetNodeRunner ignored: node %s status=%v", n.ID, n.status)
		return false

	default:
		// 혹시 모를 새 상태 대비
		Log.Warnf("SetNodeRunner unknown status: node %s status=%v", n.ID, n.status)
		return false
	}
}

func (dag *Dag) SetNodeRunners(m map[string]Runnable) (applied int, missing, skipped []string) {
	dag.mu.RLock()
	defer dag.mu.RUnlock()

	for id, r := range m {
		// 실행 노드가 아닌 특수 노드 방어
		if id == StartNode || id == EndNode {
			skipped = append(skipped, id)
			continue
		}

		n := dag.nodes[id]
		if n == nil {
			missing = append(missing, id)
			continue
		}

		n.mu.Lock()
		switch n.status {
		case NodeStatusPending:
			// TODO 백워드 호환을 위해서 넣어둠. 향후 삭제하는 방향으로 진행해야 함.
			// 사실 안전하지 않음.
			n.RunCommand = r
			// 반드시 atomic.Value에는 *runnerSlot 형태로 저장 (panic 방지)
			n.runnerStore(r)
			applied++

		case NodeStatusRunning, NodeStatusSucceeded, NodeStatusFailed, NodeStatusSkipped:
			// 이미 실행 중/완료/실패/스킵된 노드는 건너뜀 (원자성 보장)
			skipped = append(skipped, id)

		default:
			// 미래에 상태가 늘어나도 여기로 들어오면 “안전하게” 건너뜀
			skipped = append(skipped, id)
		}
		n.mu.Unlock()
	}
	return
}

// InitDagWithOptions creates and initializes a new DAG with options.
//
//nolint:unused // This function is intentionally left for future use.
func InitDagWithOptions(options ...DagOption) (*Dag, error) {
	dag := NewDagWithOptions(options...)
	if dag == nil {
		return nil, fmt.Errorf("failed to run NewDag")
	}
	return dag.StartDag()
}

// WithTimeout 타임아웃 설정 옵션을 반환
//
//nolint:unused // This function is intentionally left for future use.
func WithTimeout(timeout time.Duration) DagOption {
	return func(dag *Dag) {
		dag.Timeout = timeout
		dag.bTimeout = true
	}
}

// WithChannelBuffers 채널 버퍼 설정 옵션을 반환
//
//nolint:unused // This function is intentionally left for future use.
func WithChannelBuffers(minBuffer, maxBuffer, statusBuffer int) DagOption {
	return func(dag *Dag) {
		dag.Config.MinChannelBuffer = minBuffer
		dag.Config.MaxChannelBuffer = maxBuffer
		dag.Config.StatusBuffer = statusBuffer
	}
}

// WithWorkerPool 워커 풀 설정 옵션을 반환
//
//nolint:unused // This function is intentionally left for future use.
func WithWorkerPool(size int) DagOption {
	return func(dag *Dag) {
		dag.Config.WorkerPoolSize = size
	}
}

// ==================== DagWorkerPool 메서드 ====================

// NewDagWorkerPool 새로운 워커 풀을 생성
func NewDagWorkerPool(limit int) *DagWorkerPool {
	pool := &DagWorkerPool{
		workerLimit: limit,
		taskQueue:   make(chan func(), limit*2), // 버퍼 크기는 워커 수의 2배
	}

	// 워커 고루틴 시작
	for i := 0; i < limit; i++ { //nolint:intrange
		pool.wg.Add(1)
		go func() {
			defer pool.wg.Done()
			for task := range pool.taskQueue {
				task()
			}
		}()
	}

	return pool
}

// Submit 작업을 워커 풀에 보냄
func (p *DagWorkerPool) Submit(task func()) {
	p.taskQueue <- task
}

// Close 워커 풀을 종료
func (p *DagWorkerPool) Close() {
	close(p.taskQueue)
	p.wg.Wait()
}

// ==================== Dag 메서드 ====================

// StartDag initializes the DAG with a start node.
func (dag *Dag) StartDag() (*Dag, error) {
	dag.mu.Lock()
	defer dag.mu.Unlock()

	// logErr 헬퍼 함수 정의: 에러 로그에 추가하고 reportError 호출
	logErr := func(err error) error {
		dag.errLogs = append(dag.errLogs, &systemError{StartDag, err}) // 필요에 따라 타입을 수정
		dag.reportError(err)
		return err
	}

	// StartNode 생성 및 에러 처리
	if dag.StartNode = dag.createNode(StartNode); dag.StartNode == nil {
		return nil, logErr(fmt.Errorf("failed to create start node"))
	}

	// 새 제네릭 SafeChannel 생성 후, 시작 노드의 parentVertex에 추가
	safeChan := NewSafeChannelGen[runningStatus](dag.Config.MinChannelBuffer)
	dag.StartNode.parentVertex = append(dag.StartNode.parentVertex, safeChan)
	return dag, nil
}

// reportError reports an error to the error channel.
func (dag *Dag) reportError(err error) {
	select {
	case dag.Errors <- err:
		// 에러가 성공적으로 전송됨
	default:
		// 채널이 가득 찬 경우 로그에 기록
		Log.Printf("Error channel full, dropping error: %v", err)
	}
}

// collectErrors collects errors from the error channel.
func (dag *Dag) collectErrors(ctx context.Context) []error {
	var errors []error

	// 타임아웃 설정
	//nolint:mnd // 추후 수정하자.
	timeout := time.After(5 * time.Second)

	for {
		select {
		case err := <-dag.Errors:
			errors = append(errors, err)
		case <-timeout:
			return errors
		case <-ctx.Done():
			return errors
		default:
			if len(errors) > 0 {
				// 일정 시간 동안 새 에러가 없으면 반환
				select {
				case err := <-dag.Errors:
					errors = append(errors, err)
				case <-time.After(100 * time.Millisecond):
					return errors
				}
			} else {
				// 에러가 없으면 짧게 대기
				//nolint:mnd // 추후 수정하자.
				time.Sleep(10 * time.Millisecond)
			}
		}
	}
}

// createEdge creates an Edge with safety mechanisms.
func (dag *Dag) createEdge(parentID, childID string) (*Edge, createEdgeErrorType) {
	if utils.IsEmptyString(parentID) || utils.IsEmptyString(childID) {
		return nil, Fault
	}

	// 이미 존재하는 엣지 확인
	if edgeExists(dag.Edges, parentID, childID) {
		return nil, Exist
	}

	edge := &Edge{
		parentID:   parentID,
		childID:    childID,
		safeVertex: NewSafeChannelGen[runningStatus](dag.Config.MinChannelBuffer), // 제네릭 SafeChannel 을 사용하여 안전한 채널 생성
	}

	dag.Edges = append(dag.Edges, edge)
	return edge, Create
}

// closeChannels safely closes all channels in the DAG.
func (dag *Dag) closeChannels() {
	for _, edge := range dag.Edges {
		if edge.safeVertex != nil {
			if err := edge.safeVertex.Close(); err != nil {
				Log.Warnf("Failed to close edge channel [%s -> %s]: %v", edge.parentID, edge.childID, err)
			} else {
				Log.Infof("Closed edge channel [%s -> %s]", edge.parentID, edge.childID)
			}
		}
	}

	if dag.NodesResult != nil {
		if err := dag.NodesResult.Close(); err != nil {
			Log.Warnf("Failed to close NodesResult channel: %v", err)
		} else {
			Log.Info("Closed NodesResult channel")
		}
	}

	for i, sc := range dag.nodeResult {
		if sc == nil {
			continue
		}
		if err := sc.Close(); err != nil {
			Log.Warnf("Failed to close nodeResult[%d] channel: %v", i, err)
		} else {
			Log.Infof("Closed nodeResult[%d] channel", i)
		}
	}
}

// getSafeVertex returns the channel for the specified parent and child nodes.
//
//nolint:unused // This function is intentionally left for future use.
func (dag *Dag) getSafeVertex(parentID, childID string) *SafeChannel[runningStatus] {
	for _, v := range dag.Edges {
		if v.parentID == parentID && v.childID == childID {
			return v.safeVertex
		}
	}
	return nil
}

// Progress returns the progress of the DAG execution.
func (dag *Dag) Progress() float64 {
	nodeCount := atomic.LoadInt64(&dag.nodeCount)
	if nodeCount == 0 {
		return 0.0
	}
	completedCount := atomic.LoadInt64(&dag.completedCount)
	return float64(completedCount) / float64(nodeCount)
}

// CreateNode creates a pointer to a new node with thread safety.
func (dag *Dag) CreateNode(id string) *Node {
	dag.mu.Lock()
	defer dag.mu.Unlock()

	return dag.createNode(id)
}

func (dag *Dag) CreateNodeWithTimeOut(id string, bTimeOut bool, ti time.Duration) *Node {
	dag.mu.Lock()
	defer dag.mu.Unlock()
	if bTimeOut {
		return dag.createNodeWithTimeOut(id, ti)
	}
	return dag.createNode(id)
}

// createNode is the internal implementation of CreateNode.
func (dag *Dag) createNode(id string) *Node {
	// 이미 해당 id의 노드가 존재하면 nil 반환
	if _, exists := dag.nodes[id]; exists {
		return nil
	}

	var node *Node
	if dag.ContainerCmd != nil {
		node = createNode(id, dag.ContainerCmd)
	} else {
		node = createNodeWithID(id)
	}
	//node.runnerStore(dag.ContainerCmd)
	// 추가 초기 스토어: 기본 러너가 없어도 &runnerSlot{}로 non-nil 보장
	node.runnerStore(nil)
	node.parentDag = dag
	dag.nodes[id] = node

	// StartNode 나 EndNode 가 아닌 경우에만 노드 카운트 증가
	if id != StartNode && id != EndNode {
		atomic.AddInt64(&dag.nodeCount, 1)
	}

	return node
}

func (dag *Dag) createNodeWithTimeOut(id string, ti time.Duration) *Node {
	// 이미 해당 id의 노드가 존재하면 nil 반환
	if _, exists := dag.nodes[id]; exists {
		return nil
	}

	var node *Node
	if dag.ContainerCmd != nil {
		// dag.ContainerCmd 이건 모든 노드에 적용되게 하는 옵션이라서 이렇게 함.
		node = createNode(id, dag.ContainerCmd)
	} else {
		node = createNodeWithID(id)
	}

	// 실행 시점 반영 기본: nil 저장
	node.runnerStore(nil)

	node.bTimeout = true
	node.Timeout = ti
	node.parentDag = dag
	dag.nodes[id] = node

	// StartNode 나 EndNode 가 아닌 경우에만 노드 카운트 증가
	if id != StartNode && id != EndNode {
		atomic.AddInt64(&dag.nodeCount, 1)
	}

	return node
}

// AddEdge adds an edge between two nodes with improved error handling.
func (dag *Dag) AddEdge(from, to string) error {
	// 에러 로그를 기록하고 반환하는 클로저 함수
	logErr := func(err error) error {
		dag.errLogs = append(dag.errLogs, &systemError{AddEdge, err})
		dag.reportError(err) // 에러 채널에 보고
		return err
	}

	// 입력값 검증
	if from == to {
		return logErr(fmt.Errorf("from-node and to-node are same"))
	}
	if utils.IsEmptyString(from) {
		return logErr(fmt.Errorf("from-node is empty string"))
	}
	if utils.IsEmptyString(to) {
		return logErr(fmt.Errorf("to-node is empty string"))
	}

	dag.mu.Lock()
	defer dag.mu.Unlock()

	// 노드를 가져오거나 생성하는 클로저 함수
	getOrCreateNode := func(id string) (*Node, error) {
		if node := dag.nodes[id]; node != nil {
			return node, nil
		}
		node := dag.createNode(id)
		if node == nil {
			return nil, logErr(fmt.Errorf("%s: createNode returned nil", id))
		}
		return node, nil
	}

	fromNode, err := getOrCreateNode(from)
	if err != nil {
		return err
	}
	toNode, err := getOrCreateNode(to)
	if err != nil {
		return err
	}

	// 자식과 부모 관계 설정
	fromNode.children = append(fromNode.children, toNode)
	toNode.parent = append(toNode.parent, fromNode)

	// 엣지 생성 및 검증
	edge, check := dag.createEdge(fromNode.ID, toNode.ID)
	if check == Fault || check == Exist {
		return logErr(fmt.Errorf("edge cannot be created"))
	}
	if edge == nil {
		return logErr(fmt.Errorf("vertex is nil"))
	}

	fromNode.childrenVertex = append(fromNode.childrenVertex, edge.safeVertex)
	toNode.parentVertex = append(toNode.parentVertex, edge.safeVertex)
	return nil
}

// AddEdgeIfNodesExist adds an edge only if both nodes already exist.
func (dag *Dag) AddEdgeIfNodesExist(from, to string) error {
	// 에러 로그를 기록하고 반환하는 클로저 함수
	logErr := func(err error) error {
		dag.errLogs = append(dag.errLogs, &systemError{AddEdgeIfNodesExist, err})
		dag.reportError(err) // 에러 채널에 보고
		return err
	}

	// 입력값 검증
	if from == to {
		return logErr(fmt.Errorf("from-node and to-node are same"))
	}
	if utils.IsEmptyString(from) {
		return logErr(fmt.Errorf("from-node is empty string"))
	}
	if utils.IsEmptyString(to) {
		return logErr(fmt.Errorf("to-node is empty string"))
	}

	dag.mu.Lock()
	defer dag.mu.Unlock()

	// 노드를 가져오는 클로저 함수: 노드가 없으면 에러 리턴
	getNode := func(id string) (*Node, error) {
		if node := dag.nodes[id]; node != nil {
			return node, nil
		}
		return nil, logErr(fmt.Errorf("%s: node does not exist", id))
	}

	fromNode, err := getNode(from)
	if err != nil {
		return err
	}
	toNode, err := getNode(to)
	if err != nil {
		return err
	}

	// 자식과 부모 관계 설정
	fromNode.children = append(fromNode.children, toNode)
	toNode.parent = append(toNode.parent, fromNode)

	// 엣지 생성 및 검증
	edge, check := dag.createEdge(fromNode.ID, toNode.ID)
	if check == Fault || check == Exist {
		return logErr(fmt.Errorf("edge cannot be created"))
	}
	if edge == nil {
		return logErr(fmt.Errorf("vertex is nil"))
	}

	fromNode.childrenVertex = append(fromNode.childrenVertex, edge.safeVertex)
	toNode.parentVertex = append(toNode.parentVertex, edge.safeVertex)
	return nil
}

// addEndNode adds an edge to the end node.
func (dag *Dag) addEndNode(fromNode, toNode *Node) error {
	// logErr 헬퍼 함수: 에러 발생 시 errLogs에 기록하고, reportError 호출
	logErr := func(err error) error {
		dag.errLogs = append(dag.errLogs, &systemError{addEndNode, err})
		dag.reportError(err)
		return err
	}

	// 입력 노드 검증
	if fromNode == nil {
		return logErr(fmt.Errorf("fromNode is nil"))
	}
	if toNode == nil {
		return logErr(fmt.Errorf("toNode is nil"))
	}

	// 부모-자식 관계 설정
	fromNode.children = append(fromNode.children, toNode)
	toNode.parent = append(toNode.parent, fromNode)

	// 엣지 생성 및 체크
	edge, check := dag.createEdge(fromNode.ID, toNode.ID)
	if check == Fault || check == Exist {
		return logErr(fmt.Errorf("edge cannot be created"))
	}
	if edge == nil {
		return logErr(fmt.Errorf("vertex is nil"))
	}

	// 엣지의 vertex를 양쪽 노드에 추가
	fromNode.childrenVertex = append(fromNode.childrenVertex, edge.safeVertex)
	toNode.parentVertex = append(toNode.parentVertex, edge.safeVertex)

	return nil
}

// FinishDag finalizes the DAG by connecting end nodes and validating the structure.
func (dag *Dag) FinishDag() error {
	dag.mu.Lock()
	defer dag.mu.Unlock()

	// 에러 로그를 기록하고 반환하는 클로저 함수 (finishDag 에러 타입 사용)
	logErr := func(err error) error {
		dag.errLogs = append(dag.errLogs, &systemError{FinishDag, err})
		dag.reportError(err) // 에러 채널에 보고
		return err
	}

	// 이미 검증이 완료된 경우
	if dag.validated {
		return logErr(fmt.Errorf("validated is already set to true"))
	}

	// 노드가 하나도 없는 경우
	if len(dag.nodes) == 0 {
		return logErr(fmt.Errorf("no node"))
	}

	// 안전한 반복을 위해 노드 슬라이스를 생성 (맵의 구조 변경 방지를 위해)
	nodes := make([]*Node, 0, len(dag.nodes))
	for _, n := range dag.nodes {
		nodes = append(nodes, n)
	}

	// 종료 노드 생성 및 초기화
	dag.EndNode = dag.createNode(EndNode)
	if dag.EndNode == nil {
		return logErr(fmt.Errorf("failed to create end node"))
	}
	// TODO 일단 여기서 무조건 성공을 넣어 버리는데 end 노드에서 향후 리소스 초기화 과정을 거쳐야 하기때문에 이 부분은 수정해줘야 한다.
	dag.EndNode.SetSucceed(true)

	// 각 노드에 대해 검증 및 종료 노드로의 연결 작업 수행
	for _, n := range nodes {
		// 부모와 자식이 없는 고립된 노드가 있는 경우
		if len(n.children) == 0 && len(n.parent) == 0 {
			if len(nodes) == 1 {
				// 노드가 단 하나일 경우, 반드시 시작 노드여야 함.
				if n.ID != StartNode {
					return logErr(fmt.Errorf("invalid node: only node is not the start node"))
				}
			} else {
				return logErr(fmt.Errorf("node '%s' has no parent and no children", n.ID))
			}
		}

		// 종료 노드가 아니면서 자식이 없는 경우, 종료 노드와 연결
		if n.ID != EndNode && len(n.children) == 0 {
			if err := dag.addEndNode(n, dag.EndNode); err != nil {
				return logErr(fmt.Errorf("addEndNode failed for node '%s': %w", n.ID, err))
			}
		}
	}

	// 사이클 검사
	if DetectCycle(dag) {
		return logErr(fmt.Errorf("cycle detected in DAG"))
	}

	// 검증 완료 플래그 설정
	dag.validated = true
	return nil
}

// visitReset resets the visited status of all nodes.
//
//nolint:unused // This function is intentionally left for future use.
func (dag *Dag) visitReset() map[string]bool {
	dag.mu.RLock()
	defer dag.mu.RUnlock()

	size := len(dag.nodes)
	if size <= 0 {
		return nil
	}

	visited := make(map[string]bool, len(dag.nodes))

	for k := range dag.nodes {
		visited[k] = false
	}
	return visited
}

// ConnectRunner connects runner functions to all nodes.
func (dag *Dag) ConnectRunner() bool {
	dag.mu.RLock()
	defer dag.mu.RUnlock()

	n := len(dag.nodes)
	if n < 1 {
		return false
	}
	for _, v := range dag.nodes {
		connectRunner(v)
	}
	return true
}

// GetReady prepares the DAG for execution with worker pool.
func (dag *Dag) GetReady(ctx context.Context) bool {
	dag.mu.Lock()
	defer dag.mu.Unlock()

	n := len(dag.nodes)
	if n < 1 {
		return false
	}

	// TODO 이거 생각해보자. -> 워커 풀.
	// 워커 풀 초기화
	maxWorkers := minInt(n, dag.Config.WorkerPoolSize)
	dag.workerPool = NewDagWorkerPool(maxWorkers)

	// 각 노드별로 SafeChannel[*printStatus]를 생성하여 safeChs 슬라이스에 저장한다.
	var safeChs []*SafeChannel[*printStatus]

	for _, v := range dag.nodes {
		nd := v // 캡처 문제 방지
		// node 에서 내부적으로 처리할때 그 결과를 받는 채널.
		sc := NewSafeChannelGen[*printStatus](dag.Config.MinChannelBuffer)
		safeChs = append(safeChs, sc)

		// 워커 풀에 작업 제출
		dag.workerPool.Submit(func() {
			select {
			case <-ctx.Done():
				// sc.Close()
				return
			default:
				nd.runner(ctx, sc)
			}
		})
	}

	if dag.nodeResult != nil {
		return false
	}
	dag.nodeResult = safeChs
	return true
}

// Start initiates the DAG execution.
func (dag *Dag) Start() bool {
	if len(dag.StartNode.parentVertex) != 1 {
		return false
	}

	sc := dag.StartNode.parentVertex[0]
	if !sc.Send(Start) {
		// Send 실패시 적절한 로그 출력 혹은 상태 변경 처리.
		Log.Warnf("Failed to send Start status on safe channel for start node")
		dag.StartNode.SetSucceed(false)
		return false
	}
	dag.StartNode.SetSucceed(true)
	return true
}

// Wait waits for the DAG execution to complete.
func (dag *Dag) Wait(ctx context.Context) bool {
	// DAG 종료 시 채널들을 안전하게 닫는다.
	defer dag.closeChannels()

	// 워커 풀 종료
	if dag.workerPool != nil {
		defer dag.workerPool.Close()
	}

	// 컨텍스트에서 타임아웃 설정
	var waitCtx context.Context
	var cancel context.CancelFunc
	if dag.bTimeout {
		waitCtx, cancel = context.WithTimeout(ctx, dag.Timeout)
	} else {
		waitCtx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	// merge 의 결과를 받을 채널을 생성한다.
	mergeResult := make(chan bool, 1)
	// 고루틴에서 merge 함수를 실행하고, 그 결과를 mergeResult 채널에 보낸다.
	go func() {
		mergeResult <- dag.merge(waitCtx)
	}()

	for {
		select {
		case ok := <-mergeResult:
			// merge 함수가 완료되어 결과를 반환한 경우,
			// false 이면 병합 작업에 실패한 것이므로 false 리턴.
			if !ok {
				return false
			}
			// merge 함수 결과가 true 면 계속 진행한다.
		case c, ok := <-dag.NodesResult.GetChannel():
			if !ok {
				// 채널이 종료되면 실패 처리.
				return false
			}
			// EndNode 에 대한 상태만 체크함.
			if c.nodeID == EndNode {
				if c.rStatus == PreflightFailed ||
					c.rStatus == InFlightFailed ||
					c.rStatus == PostFlightFailed {
					return false
				}
				if c.rStatus == FlightEnd {
					return true
				}
			}
		case <-waitCtx.Done():
			Log.Printf("DAG execution timed out or canceled: %v", waitCtx.Err())
			return false
		}
	}
}

// detectCycleDFS detects cycles using DFS.
func detectCycleDFS(node *Node, visited, recStack map[string]bool) bool {
	if recStack[node.ID] {
		return true
	}
	if visited[node.ID] {
		return false
	}
	visited[node.ID] = true
	recStack[node.ID] = true

	for _, child := range node.children {
		if detectCycleDFS(child, visited, recStack) {
			return true
		}
	}

	recStack[node.ID] = false
	return false
}

// DetectCycle checks if the DAG contains a cycle.
func DetectCycle(dag *Dag) bool {
	// copyDag 는 최소 정보만 복사하므로, 사이클 검사에 적합
	newNodes, _ := copyDag(dag)
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	// 새로 복사된 노드들을 대상으로 DFS 를 수행
	for _, node := range newNodes {
		if !visited[node.ID] {
			if detectCycleDFS(node, visited, recStack) {
				return true
			}
		}
	}
	return false
}

// connectRunner connects a runner function to a node.
func connectRunner(n *Node) {
	n.runner = func(ctx context.Context, result *SafeChannel[*printStatus]) {
		// 헬퍼 함수: printStatus 값을 복사해 반환, 지우지 말것.
		// 여기서 중요한 사항.
		// 아래 코드를 보면 printStatus 새로운 복사본을 하나 만들어서 값을 넣어주는데, 포인터를 입력파라미터로 받았다.
		// 그래서 만약 포인터나 슬라이스 등 참조형 필드를 포함한다면 얇은 복사가 이루어져서 잘못된 결과가 발생한다.
		// 하지만, rStatus 는 int 이고 nodeId 는 string 이라서 즉, 기본형이라서 복사가 이루어진다. 따라서 원본의 값이 변경된다고 해도 해당 복사본의 값의 변경은 일어나지 않는다.
		copyStatus := func(ps *printStatus) *printStatus {
			return &printStatus{
				rStatus: ps.rStatus,
				nodeID:  ps.nodeID,
			}
		}

		// 부모 노드 상태 확인
		if !n.CheckParentsStatus() {
			ps := newPrintStatus(PostFlightFailed, n.ID)
			// 복사본을 만들어 SafeChannel 에 전송
			result.Send(copyStatus(ps))
			n.SetStatus(NodeStatusSkipped)
			n.notifyChildren(Failed)
			releasePrintStatus(ps)
			return
		}

		n.SetStatus(NodeStatusRunning)

		// preFlight 단계 실행
		ps := preFlight(ctx, n)
		result.Send(copyStatus(ps))
		if ps.rStatus == PreflightFailed {
			n.SetStatus(NodeStatusFailed)
			n.notifyChildren(Failed)
			releasePrintStatus(ps)
			return
		}
		releasePrintStatus(ps)

		// inFlight 단계 실행
		ps = inFlight(n)
		result.Send(copyStatus(ps))
		if ps.rStatus == InFlightFailed {
			n.SetStatus(NodeStatusFailed)
			n.notifyChildren(Failed)
			releasePrintStatus(ps)
			return
		}
		releasePrintStatus(ps)

		// postFlight 단계 실행
		ps = postFlight(n)
		result.Send(copyStatus(ps))
		if ps.rStatus == PostFlightFailed {
			n.SetStatus(NodeStatusFailed)
		} else {
			n.SetStatus(NodeStatusSucceeded)
		}
		releasePrintStatus(ps)
	}
}

// insertSafe inserts a value into a slice at the specified index. 지금은 사용하지 않지만 지우지 말것.
//
//nolint:unused // This function is intentionally left for future use.
func insertSafe(a []*SafeChannel[*printStatus], index int, value *SafeChannel[*printStatus]) []*SafeChannel[*printStatus] {
	if len(a) == index { // 빈 슬라이스이거나 마지막 요소 뒤에 삽입하는 경우
		return append(a, value)
	}
	a = append(a[:index+1], a[index:]...)
	a[index] = value
	return a
}

// merge merges all status channels using fan-in pattern.
func (dag *Dag) merge(ctx context.Context) bool {
	// 입력 SafeChannel 슬라이스가 비어 있으면 그대로 종료함.
	if len(dag.nodeResult) < 1 {
		return false
	}

	return fanIn(ctx, dag.nodeResult, dag.NodesResult)
}

// fanIn merges multiple channels into one.
func fanIn(ctx context.Context, channels []*SafeChannel[*printStatus], merged *SafeChannel[*printStatus]) bool {
	var wg sync.WaitGroup
	var canceled int32 // 0: 정상, 1: ctx.Done 발생

	// 각 SafeChannel 의 내부 채널에서 값을 읽어와 merged SafeChannel 에 블로킹 전송
	for _, sc := range channels {
		wg.Add(1)
		go func(sc *SafeChannel[*printStatus]) {
			defer wg.Done()
			// 내부 채널을 순회
			for val := range sc.GetChannel() {
				select {
				case <-ctx.Done():
					atomic.StoreInt32(&canceled, 1)
					return
				case merged.GetChannel() <- val:
					// 값 전송 성공하면 계속 진행
				}
			}
		}(sc)
	}

	wg.Wait()

	// cancellation 플래그가 설정되어 있으면 false, 아니면 true 리턴
	return atomic.LoadInt32(&canceled) == 0
}

// min returns the minimum of two integers.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func CopyDag(original *Dag, newID string) *Dag {
	if original == nil {
		return nil
	}
	if utils.IsEmptyString(newID) {
		return nil
	}

	copied := &Dag{
		ID:           newID,
		Timeout:      original.Timeout,
		bTimeout:     original.bTimeout,
		ContainerCmd: original.ContainerCmd,
		validated:    original.validated,
		NodesResult:  NewSafeChannelGen[*printStatus](original.Config.MaxChannelBuffer),
	}

	// 노드와 간선 복사
	newNodes, newEdges := copyDag(original)
	copied.nodes = newNodes
	copied.Edges = newEdges

	// 노드에 새 DAG 참조를 설정하고 시작/종료 노드 확인
	for _, node := range newNodes {
		node.parentDag = copied
		switch node.ID {
		case StartNode:
			copied.StartNode = node
		case EndNode:
			copied.EndNode = node
		}
	}

	return copied
}

// copyDag dag 를 복사함. 필요한것만 복사함.
func copyDag(original *Dag) (map[string]*Node, []*Edge) {
	// 원본에 노드가 없으면 nil 반환
	if len(original.nodes) == 0 {
		return nil, nil
	}

	// 1. 노드의 기본 정보(Id)만 복사한 새 맵 생성
	newNodes := make(map[string]*Node, len(original.nodes))
	for _, n := range original.nodes {
		// 필요한 최소한의 정보만 복사
		newNode := &Node{
			ID: n.ID, // ✅ Node 구조체가 Id로 되어 있다면 그대로 유지
			// 기타 필드는 cycle 검증에 필요하지 않으므로 생략
		}
		newNodes[newNode.ID] = newNode
	}

	// 2. 원본 노드의 부모/자식 관계를 이용하여 새 노드들의 포인터 연결
	for _, n := range original.nodes {
		newNode := newNodes[n.ID]
		// 부모 노드 연결
		for _, parent := range n.parent {
			if copiedParent, ok := newNodes[parent.ID]; ok {
				newNode.parent = append(newNode.parent, copiedParent)
			}
		}
		// 자식 노드 연결
		for _, child := range n.children {
			if copiedChild, ok := newNodes[child.ID]; ok {
				newNode.children = append(newNode.children, copiedChild)
			}
		}
	}

	// 3. 간선(Edge) 복사: parentID, childID만 복사
	newEdges := make([]*Edge, len(original.Edges))
	for i, e := range original.Edges {
		newEdges[i] = &Edge{
			parentID: e.parentID, // ✅ 수정됨
			childID:  e.childID,  // ✅ 수정됨
			// vertex 등 기타 정보는 cycle 검증에 필요하지 않으므로 생략
		}
	}

	return newNodes, newEdges
}

func edgeExists(edges []*Edge, parentID, childID string) bool {
	for _, edge := range edges {
		if edge.parentID == parentID && edge.childID == childID {
			return true
		}
	}
	return false
}
