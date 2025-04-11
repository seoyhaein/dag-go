package dag_go

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/seoyhaein/utils"
	"sync"
	"sync/atomic"
	"time"
)

// TODO 아래 코드 사용하는 부분 면밀히 검토해야함. 중요. 우선 처리할것.
// NewSafeChannelGen
// channel 들에 대한 close 를 다 확인한다.

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
const noNodeId = "-1"

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
		nodeId  string
	}
)

// createEdgeErrorType 0 if created, 1 if exists, 2 if error.
type createEdgeErrorType int

// Dag (Directed Acyclic Graph) is an acyclic graph, not a cyclic graph.
// In other words, there is no cyclic cycle in the DAG algorithm, and it has only one direction.
type Dag struct {
	Pid   string // TODO 이거 삭제해도 될것 같은데..
	Id    string
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

	ContainerCmd Runnable

	// 추가된 필드
	Config         DagConfig
	workerPool     *DagWorkerPool
	nodeCount      int64        // 노드 수를 원자적으로 추적
	completedCount int64        // 완료된 노드 수를 원자적으로 추적
	mu             sync.RWMutex // 맵 접근을 보호하기 위한 뮤텍스
}

// Edge is a channel. It has the same meaning as the connecting line connecting the parent and child nodes.
type Edge struct {
	parentId   string
	childId    string
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
		Id:          uuid.NewString(),
		NodesResult: NewSafeChannelGen[*printStatus](config.MaxChannelBuffer),
		Config:      config,
		Errors:      make(chan error, config.MaxChannelBuffer), // 에러 채널 초기화
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

// InitDagWithOptions creates and initializes a new DAG with options.
func InitDagWithOptions(options ...DagOption) (*Dag, error) {
	dag := NewDagWithOptions(options...)
	if dag == nil {
		return nil, fmt.Errorf("failed to run NewDag")
	}
	return dag.StartDag()
}

// WithTimeout 타임아웃 설정 옵션을 반환
func WithTimeout(timeout time.Duration) DagOption {
	return func(dag *Dag) {
		dag.Timeout = timeout
		dag.bTimeout = true
	}
}

// WithChannelBuffers 채널 버퍼 설정 옵션을 반환
func WithChannelBuffers(min, max, status int) DagOption {
	return func(dag *Dag) {
		dag.Config.MinChannelBuffer = min
		dag.Config.MaxChannelBuffer = max
		dag.Config.StatusBuffer = status
	}
}

// WithWorkerPool 워커 풀 설정 옵션을 반환
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
	for i := 0; i < limit; i++ {
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

	if dag.StartNode = dag.createNode(StartNode); dag.StartNode == nil {
		return nil, fmt.Errorf("failed to create start node")
	}
	// 새 제네릭 SafeChannel 을 생성하고, 그 내부 채널을 시작 노드의 parentVertex 에 추가함.
	safeChan := NewSafeChannelGen[runningStatus](dag.Config.MinChannelBuffer)
	dag.StartNode.parentVertex = append(dag.StartNode.parentVertex, safeChan)
	return dag, nil
}

// SetContainerCmd sets the container command for the DAG.
func (dag *Dag) SetContainerCmd(r Runnable) {
	dag.ContainerCmd = r
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
				time.Sleep(10 * time.Millisecond)
			}
		}
	}
}

// createEdge creates an Edge with safety mechanisms.
func (dag *Dag) createEdge(parentId, childId string) (*Edge, createEdgeErrorType) {
	if utils.IsEmptyString(parentId) || utils.IsEmptyString(childId) {
		return nil, Fault
	}

	// 이미 존재하는 엣지 확인
	if edgeExists(dag.Edges, parentId, childId) {
		return nil, Exist
	}

	edge := &Edge{
		parentId:   parentId,
		childId:    childId,
		safeVertex: NewSafeChannelGen[runningStatus](dag.Config.MinChannelBuffer), // 제네릭 SafeChannel 을 사용하여 안전한 채널 생성
	}

	dag.Edges = append(dag.Edges, edge)
	return edge, Create
}

// closeChannels safely closes all channels in the DAG.
func (dag *Dag) closeChannels() {
	for _, edge := range dag.Edges {
		if edge.safeVertex != nil {
			cErr := edge.safeVertex.Close()
			if cErr != nil {
				Log.Printf("Error closing channel for edge %s -> %s: %v", edge.parentId, edge.childId, cErr)
			} else {
				Log.Printf("Closed channel for edge %s -> %s", edge.parentId, edge.childId)
			}
		}
	}
}

// getSafeVertex returns the channel for the specified parent and child nodes.
func (dag *Dag) getSafeVertex(parentId, childId string) *SafeChannel[runningStatus] {
	for _, v := range dag.Edges {
		if v.parentId == parentId && v.childId == childId {
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
		node = createNodeWithId(id)
	}

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
		node = createNode(id, dag.ContainerCmd)
	} else {
		node = createNodeWithId(id)
	}

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
	edge, check := dag.createEdge(fromNode.Id, toNode.Id)
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
	edge, check := dag.createEdge(fromNode.Id, toNode.Id)
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
	// 입력 노드 검증
	if fromNode == nil {
		return fmt.Errorf("fromNode is nil")
	}
	if toNode == nil {
		return fmt.Errorf("toNode is nil")
	}

	// 부모-자식 관계 설정
	fromNode.children = append(fromNode.children, toNode)
	toNode.parent = append(toNode.parent, fromNode)

	// 엣지 생성 및 체크
	edge, check := dag.createEdge(fromNode.Id, toNode.Id)
	if check == Fault || check == Exist {
		return fmt.Errorf("edge cannot be created")
	}
	if edge == nil {
		return fmt.Errorf("vertex is nil")
	}

	// 엣지의 vertex 양쪽 노드에 추가
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
		dag.errLogs = append(dag.errLogs, &systemError{finishDag, err})
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
	dag.EndNode.SetSucceed(true)

	// 각 노드에 대해 검증 및 종료 노드로의 연결 작업 수행
	for _, n := range nodes {
		// 부모와 자식이 없는 고립된 노드가 있는 경우
		if len(n.children) == 0 && len(n.parent) == 0 {
			if len(nodes) == 1 {
				// 노드가 단 하나일 경우, 반드시 시작 노드여야 함.
				if n.Id != StartNode {
					return logErr(fmt.Errorf("invalid node: only node is not the start node"))
				}
			} else {
				return logErr(fmt.Errorf("node '%s' has no parent and no children", n.Id))
			}
		}

		// 종료 노드가 아니면서 자식이 없는 경우, 종료 노드와 연결
		if n.Id != EndNode && len(n.children) == 0 {
			if err := dag.addEndNode(n, dag.EndNode); err != nil {
				return logErr(fmt.Errorf("addEndNode failed for node '%s': %w", n.Id, err))
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

// TODO GetReadyT 와 GetReady 하나만 남겨놓기.

// GetReadyT prepares the DAG for execution with worker pool.
func (dag *Dag) GetReadyT(ctx context.Context) bool {
	dag.mu.Lock()
	defer dag.mu.Unlock()

	n := len(dag.nodes)
	if n < 1 {
		return false
	}

	// 워커 풀 초기화
	maxWorkers := min(n, dag.Config.WorkerPoolSize)
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
				// TODO 확인하기. 이거 지워야 할거 같은데.
				sc.Close()
				return
			default:
				nd.runner(ctx, sc)
			}
		})
	}

	// TODO 수정해줘야 함. 일단 채널 슬라이스인데 단순히 nil 체크로 올바른지 확인 필요.
	if dag.nodeResult != nil {
		return false
	}
	dag.nodeResult = safeChs
	return true
}

// GetReady prepares the DAG for execution with separate handling for start and end nodes.
func (dag *Dag) GetReady(ctx context.Context) bool {
	dag.mu.Lock()
	defer dag.mu.Unlock()

	n := len(dag.nodes)
	if n < 1 {
		return false
	}

	// 워커 풀 초기화
	maxWorkers := min(n, dag.Config.WorkerPoolSize)
	dag.workerPool = NewDagWorkerPool(maxWorkers)

	var safeChs []*SafeChannel[*printStatus]
	// 종료 노드용 SafeChannel 생성
	endCh := NewSafeChannelGen[*printStatus](dag.Config.MinChannelBuffer)
	var end *Node

	// 각 노드를 순회하며 SafeChannel 생성 및 작업 제출
	for _, v := range dag.nodes {
		// 변수 캡처를 위해 별도 변수에 할당
		node := v

		if node.Id == StartNode {
			safeCh := NewSafeChannelGen[*printStatus](dag.Config.MinChannelBuffer)
			// 시작 노드는 맨 앞에 삽입
			safeChs = insertSafe(safeChs, 0, safeCh)

			// 워커 풀에 시작 노드의 작업 제출
			dag.workerPool.Submit(func() {
				select {
				case <-ctx.Done():
					safeCh.Close()
					return
				default:
					node.runner(ctx, safeCh)
				}
			})
		}

		if node.Id != StartNode && node.Id != EndNode {
			safeCh := NewSafeChannelGen[*printStatus](dag.Config.MinChannelBuffer)
			safeChs = append(safeChs, safeCh)

			// 일반 노드 작업 제출
			dag.workerPool.Submit(func() {
				select {
				case <-ctx.Done():
					safeCh.Close()
					return
				default:
					node.runner(ctx, safeCh)
				}
			})
		}

		if node.Id == EndNode {
			end = node
		}
	}

	if end == nil {
		Log.Println("Warning: EndNode is nil")
		return false
	}

	// 종료 노드 SafeChannel 를 safeChs 슬라이스에 추가
	safeChs = append(safeChs, endCh)

	// 종료 노드 작업 제출
	dag.workerPool.Submit(func() {
		select {
		case <-ctx.Done():
			endCh.Close()
			return
		default:
			end.runner(ctx, endCh)
		}
	})

	// TODO 확인해볼것
	// 이미 runningStatus1가 설정되어 있다면 준비 실패로 처리
	if dag.nodeResult != nil {
		return false
	}
	// 최종적으로 safeChs 슬라이스를 dag.runningStatus1에 저장
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

/*
func (dag *Dag) Start() bool {
    if len(dag.StartNode.parentVertex) != 1 {
        return false
    }
    dag.StartNode.SetSucceed(true)

    // Send 결과를 받을 채널을 만든다.
    resultCh := make(chan bool, 1)

    go func(sc *SafeChannel[runningStatus], resultCh chan bool) {
        success := sc.Send(Start)
        resultCh <- success // Send 의 결과를 resultCh에 전달한다.
        // 채널 닫기는 DAG 레벨에서 관리함.
    }(dag.StartNode.parentVertex[0], resultCh)

    // 고루틴에서 받은 결과를 기다린다.
    success := <-resultCh
    if !success {
        Log.Printf("Failed to send Start status on safe channel for start node")
        dag.StartNode.SetSucceed(false)
    }
    return success
}
*/

// Wait waits for the DAG execution to complete. // TODO 실패 부분 보다 정확히 해야함. 버그 있음.
func (dag *Dag) Wait(ctx context.Context) bool {
	// DAG 종료 시 채널들을 안전하게 닫는다.
	defer dag.closeChannels()

	// TODO 여기서 NodesResult 와 nodeResult 를 닫는 것이 낳을 듯한데 생각해보자.

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

	// merged 값은 dag.RunningStatus1 내부 SafeChannel 의 채널을 통해 수신한다.
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
			if c.nodeId == EndNode {
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
			Log.Printf("DAG execution timed out or cancelled: %v", waitCtx.Err())
			return false
		}
	}
}

/*
func (dag *Dag) Wait(ctx context.Context) bool {
	// 채널 닫기는 DAG 종료 시 처리
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

	// 채널 병합
	go dag.merge(waitCtx)

	for {
		select {
		case c := <-dag.RunningStatus1:
			if c.nodeId == EndNode {
				if c.rStatus == PreflightFailed {
					return false
				}
				if c.rStatus == InFlightFailed {
					return false
				}
				if c.rStatus == PostFlightFailed {
					return false
				}
				if c.rStatus == FlightEnd {
					return true
				}
			}
		case <-waitCtx.Done():
			Log.Printf("DAG execution timed out or cancelled: %v", waitCtx.Err())
			return false
		}
	}
}
*/

// detectCycleDFS detects cycles using DFS.
func detectCycleDFS(node *Node, visited, recStack map[string]bool) bool {
	if recStack[node.Id] {
		return true
	}
	if visited[node.Id] {
		return false
	}
	visited[node.Id] = true
	recStack[node.Id] = true

	for _, child := range node.children {
		if detectCycleDFS(child, visited, recStack) {
			return true
		}
	}

	recStack[node.Id] = false
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
		if !visited[node.Id] {
			if detectCycleDFS(node, visited, recStack) {
				return true
			}
		}
	}
	return false
}

// connectRunner connects a runner function to a node.
/*func connectRunner(n *Node) {
	n.runner = func(ctx context.Context, result chan *printStatus) {
		defer close(result)

		// 헬퍼 함수: printStatus 값을 복사해 반환
		copyStatus := func(ps *printStatus) *printStatus {
			return &printStatus{
				rStatus: ps.rStatus,
				nodeId:  ps.nodeId,
			}
		}

		// 부모 노드 상태 확인
		if !n.CheckParentsStatus() {
			ps := newPrintStatus(PostFlightFailed, n.Id)
			// 복사본을 만들어 채널에 전달
			result <- copyStatus(ps)
			releasePrintStatus(ps)
			return
		}

		n.SetStatus(NodeStatusRunning)

		// preFlight 단계 실행
		ps := preFlight(ctx, n)
		result <- copyStatus(ps)
		if ps.rStatus == PreflightFailed {
			n.SetStatus(NodeStatusFailed)
			releasePrintStatus(ps)
			return
		}
		releasePrintStatus(ps)

		// inFlight 단계 실행
		ps = inFlight(n)
		result <- copyStatus(ps)
		if ps.rStatus == InFlightFailed {
			n.SetStatus(NodeStatusFailed)
			releasePrintStatus(ps)
			return
		}
		releasePrintStatus(ps)

		// postFlight 단계 실행
		ps = postFlight(n)
		result <- copyStatus(ps)
		if ps.rStatus == PostFlightFailed {
			n.SetStatus(NodeStatusFailed)
		} else {
			n.SetStatus(NodeStatusSucceeded)
		}
		releasePrintStatus(ps)
	}
}*/

func connectRunner(n *Node) {
	n.runner = func(ctx context.Context, result *SafeChannel[*printStatus]) {
		defer result.Close() // runner 종료 시 SafeChannel 의 내부 채널을 닫음 TODO 확인해야함.

		// 헬퍼 함수: printStatus 값을 복사해 반환, 지우지 말것.
		// 여기서 중요한 사항.
		// 아래 코드를 보면 printStatus 새로운 복사본을 하나 만들어서 값을 넣어주는데, 포인터를 입력파라미터로 받았다.
		// 그래서 만약 포인터나 슬라이스 등 참조형 필드를 포함한다면 얇은 복사가 이루어져서 잘못된 결과가 발생한다.
		// 하지만, rStatus 는 int 이고 nodeId 는 string 이라서 즉, 기본형이라서 복사가 이루어진다. 따라서 원본의 값이 변경된다고 해도 해당 복사본의 값의 변경은 일어나지 않는다.
		copyStatus := func(ps *printStatus) *printStatus {
			return &printStatus{
				rStatus: ps.rStatus,
				nodeId:  ps.nodeId,
			}
		}

		// 부모 노드 상태 확인
		if !n.CheckParentsStatus() {
			ps := newPrintStatus(PostFlightFailed, n.Id)
			// 복사본을 만들어 SafeChannel 에 전송
			result.Send(copyStatus(ps))
			releasePrintStatus(ps)
			return
		}

		n.SetStatus(NodeStatusRunning)

		// preFlight 단계 실행
		ps := preFlight(ctx, n)
		result.Send(copyStatus(ps))
		if ps.rStatus == PreflightFailed {
			n.SetStatus(NodeStatusFailed)
			releasePrintStatus(ps)
			return
		}
		releasePrintStatus(ps)

		// inFlight 단계 실행
		ps = inFlight(n)
		result.Send(copyStatus(ps))
		if ps.rStatus == InFlightFailed {
			n.SetStatus(NodeStatusFailed)
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

// insert inserts a value into a slice at the specified index.
/*func insert(a []chan *printStatus, index int, value chan *printStatus) []chan *printStatus {
	if len(a) == index { // nil or empty slice or after last element
		return append(a, value)
	}
	a = append(a[:index+1], a[index:]...) // index < len(a)
	a[index] = value
	return a
}*/
// insertSafe inserts a value into a slice at the specified index.
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
	// 종료 시 SafeChannel의 Close 메서드를 호출함. TODO 이거 왜 해주는지 이해 안됨.
	// defer dag.RunningStatus1.Close()

	// 입력 SafeChannel 슬라이스가 비어 있으면 그대로 종료함.
	if len(dag.nodeResult) < 1 {
		return false
	}

	// safeChs (dag.runningStatus1)들을 팬인 패턴으로 병합하여 merged SafeChannel을 얻음.
	return fanIn(ctx, dag.nodeResult, dag.NodesResult)

}

// fanIn merges multiple channels into one. TODO 여기서 채널을 그냥 퉁쳤는데 입력 채널, 출력 채널 구분해서 하는게 어떨지 고민하자.
func fanIn(ctx context.Context, channels []*SafeChannel[*printStatus], merged *SafeChannel[*printStatus]) bool {
	var wg sync.WaitGroup
	var cancelled int32 // 0: 정상, 1: ctx.Done 발생

	// 각 SafeChannel 의 내부 채널에서 값을 읽어와 merged SafeChannel 에 블로킹 전송
	for _, sc := range channels {
		wg.Add(1)
		go func(sc *SafeChannel[*printStatus]) {
			defer wg.Done()
			// 내부 채널을 순회
			for val := range sc.GetChannel() {
				select {
				case <-ctx.Done():
					atomic.StoreInt32(&cancelled, 1)
					return
				case merged.GetChannel() <- val:
					// 값 전송 성공하면 계속 진행
				}
			}
		}(sc)
	}

	// 모든 고루틴이 종료되면 merged SafeChannel 을 닫는다.
	wg.Wait()
	merged.Close()

	// cancellation 플래그가 설정되어 있으면 false, 아니면 true 리턴
	return atomic.LoadInt32(&cancelled) == 0
}

/*func fanIn(ctx context.Context, channels []chan *printStatus) chan *printStatus {
	merged := make(chan *printStatus)
	var wg sync.WaitGroup

	// 각 입력 채널에 대한 고루틴 시작
	for _, ch := range channels {
		wg.Add(1)
		go func(c chan *printStatus) {
			defer wg.Done()
			for val := range c {
				select {
				case merged <- val:
				case <-ctx.Done():
					return
				}
			}
		}(ch)
	}

	// 모든 입력 채널이 닫히면 출력 채널도 닫음
	go func() {
		wg.Wait()
		close(merged)
	}()

	return merged
}*/

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// copyDag dag 를 복사함. 필요한것만 복사함.
func copyDag(original *Dag) (map[string]*Node, []*Edge) {
	// 원본에 노드가 없으면 nil 반환
	if len(original.nodes) == 0 {
		return nil, nil
	}

	// 1. 노드의 기본 정보(ID)만 복사한 새 맵 생성
	newNodes := make(map[string]*Node, len(original.nodes))
	for _, n := range original.nodes {
		// 필요한 최소한의 정보만 복사
		newNode := &Node{
			Id: n.Id,
			// 기타 필드는 cycle 검증에 필요하지 않으므로 생략
		}
		newNodes[newNode.Id] = newNode
	}
	// 2. 원본 노드의 부모/자식 관계를 이용하여 새 노드들의 포인터 연결
	for _, n := range original.nodes {
		newNode := newNodes[n.Id]
		// 부모 노드 연결
		for _, parent := range n.parent {
			if copiedParent, ok := newNodes[parent.Id]; ok {
				newNode.parent = append(newNode.parent, copiedParent)
			}
		}
		// 자식 노드 연결
		for _, child := range n.children {
			if copiedChild, ok := newNodes[child.Id]; ok {
				newNode.children = append(newNode.children, copiedChild)
			}
		}
	}
	// 3. 간선(Edge) 복사: detectCycle 에 필요하다면 parentId와 childId만 복사
	newEdges := make([]*Edge, len(original.Edges))
	for i, e := range original.Edges {
		newEdges[i] = &Edge{
			parentId: e.parentId,
			childId:  e.childId,
			// vertex 등 기타 정보는 cycle 검증에 필요하지 않으므로 생략
		}
	}

	return newNodes, newEdges
}

func edgeExists(edges []*Edge, parentId, childId string) bool {
	for _, edge := range edges {
		if edge.parentId == parentId && edge.childId == childId {
			return true
		}
	}
	return false
}
