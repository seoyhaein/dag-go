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
	nodes   = "nodes"
	node    = "node"
	id      = "id"
	from    = "from"
	to      = "to"
	command = "command"
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

// SafeChannel 다중 송신자가 있는 채널을 안전하게 관리하는 구조체
type SafeChannel struct {
	ch     chan runningStatus
	closed bool
	mu     sync.RWMutex
}

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
	Pid   string
	Id    string
	Edges []*Edge

	nodes     map[string]*Node
	StartNode *Node
	EndNode   *Node
	validated bool

	RunningStatus chan *printStatus
	runningStatus []chan *printStatus

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
	vertex     chan runningStatus
	safeVertex *SafeChannel // 안전한 채널 추가
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
	dag := &Dag{
		nodes:         make(map[string]*Node),
		Id:            uuid.NewString(),
		RunningStatus: make(chan *printStatus, config.MaxChannelBuffer),
		Config:        config,
		Errors:        make(chan error, config.MaxChannelBuffer), // 에러 채널 초기화
	}
	return dag
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

// ==================== SafeChannel 메서드 ====================

// NewSafeChannel 새로운 SafeChannel을 생성
func NewSafeChannel(buffer int) *SafeChannel {
	return &SafeChannel{
		ch:     make(chan runningStatus, buffer),
		closed: false,
	}
}

// Send 채널에 안전하게 값을 보냄
func (sc *SafeChannel) Send(status runningStatus) bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	if sc.closed {
		return false
	}

	select {
	case sc.ch <- status:
		return true
	default:
		return false
	}
}

// Close 채널을 안전하게 닫음
func (sc *SafeChannel) Close() {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if !sc.closed {
		close(sc.ch)
		sc.closed = true
	}
}

// GetChannel 기본 채널을 반환
func (sc *SafeChannel) GetChannel() chan runningStatus {
	return sc.ch
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
	// 시작 노드에 필수 채널 추가 (parentVertex 채널 삽입)
	dag.StartNode.parentVertex = append(dag.StartNode.parentVertex, make(chan runningStatus, dag.Config.MinChannelBuffer))
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

	// 안전한 채널 사용
	safeVertex := NewSafeChannel(dag.Config.MinChannelBuffer)

	edge := &Edge{
		parentId:   parentId,
		childId:    childId,
		vertex:     safeVertex.GetChannel(),
		safeVertex: safeVertex,
	}

	dag.Edges = append(dag.Edges, edge)
	return edge, Create
}

// closeChannels safely closes all channels in the DAG.
func (dag *Dag) closeChannels() {
	for _, edge := range dag.Edges {
		if edge.safeVertex != nil {
			edge.safeVertex.Close()
		}
	}
}

// getVertex returns the channel for the specified parent and child nodes.
func (dag *Dag) getVertex(parentId, childId string) chan runningStatus {
	for _, v := range dag.Edges {
		if v.parentId == parentId {
			if v.childId == childId {
				if v.vertex != nil {
					return v.vertex
				} else {
					return nil
				}
			}
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

	fromNode.childrenVertex = append(fromNode.childrenVertex, edge.vertex)
	toNode.parentVertex = append(toNode.parentVertex, edge.vertex)
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

	// TODO 수정해줘야 함.
	fromNode.childrenVertex = append(fromNode.childrenVertex, edge.vertex)
	toNode.parentVertex = append(toNode.parentVertex, edge.vertex)
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
	fromNode.childrenVertex = append(fromNode.childrenVertex, edge.vertex)
	toNode.parentVertex = append(toNode.parentVertex, edge.vertex)

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

	var chs []chan *printStatus

	for _, v := range dag.nodes {
		nd := v // 변수 캡처 문제 방지
		ch := make(chan *printStatus, dag.Config.StatusBuffer)
		chs = append(chs, ch)

		// 작업을 워커 풀에 제출
		dag.workerPool.Submit(func() {
			// 컨텍스트 취소 시 고루틴이 정리되도록 보장
			select {
			case <-ctx.Done():
				close(ch)
				return
			default:
				nd.runner(ctx, ch)
			}
		})
	}

	if dag.runningStatus != nil {
		return false
	}
	dag.runningStatus = chs
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

	var chs []chan *printStatus
	endCh := make(chan *printStatus, dag.Config.StatusBuffer)
	var end *Node

	for _, v := range dag.nodes {
		node := v // 변수 캡처 문제 방지

		if node.Id == StartNode {
			start := make(chan *printStatus, dag.Config.StatusBuffer)
			chs = insert(chs, 0, start)

			// 작업을 워커 풀에 제출
			dag.workerPool.Submit(func() {
				// 컨텍스트 취소 시 고루틴이 정리되도록 보장
				select {
				case <-ctx.Done():
					close(start)
					return
				default:
					node.runner(ctx, start)
				}
			})
		}

		if node.Id != StartNode && node.Id != EndNode {
			ch := make(chan *printStatus, dag.Config.StatusBuffer)
			chs = append(chs, ch)

			// 작업을 워커 풀에 제출
			dag.workerPool.Submit(func() {
				// 컨텍스트 취소 시 고루틴이 정리되도록 보장
				select {
				case <-ctx.Done():
					close(ch)
					return
				default:
					node.runner(ctx, ch)
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

	chs = append(chs, endCh)

	// 작업을 워커 풀에 제출
	dag.workerPool.Submit(func() {
		// 컨텍스트 취소 시 고루틴이 정리되도록 보장
		select {
		case <-ctx.Done():
			close(endCh)
			return
		default:
			end.runner(ctx, endCh)
		}
	})

	if dag.runningStatus != nil {
		return false
	}
	dag.runningStatus = chs
	return true
}

// Start initiates the DAG execution.
func (dag *Dag) Start() bool {
	n := len(dag.StartNode.parentVertex)
	// 1 이 아니면 에러다.
	if n != 1 {
		return false
	}

	dag.StartNode.SetSucceed(true)
	go func(c chan runningStatus) {
		ch := c
		ch <- Start
		// 채널 닫기는 DAG 레벨에서 관리
	}(dag.StartNode.parentVertex[0])

	return true
}

// mergeT merges all status channels using fan-in pattern.
func (dag *Dag) merge(ctx context.Context) {
	defer close(dag.RunningStatus)

	if len(dag.runningStatus) < 1 {
		return
	}

	// 팬인 패턴 적용
	merged := fanIn(ctx, dag.runningStatus)

	// 결과 처리
	for val := range merged {
		dag.RunningStatus <- val
	}
}

// Wait waits for the DAG execution to complete. // TODO 실패 부분 보다 정확히 해야함. 버그 있음.
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
		case c := <-dag.RunningStatus:
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
func connectRunner(n *Node) {
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
}

// insert inserts a value into a slice at the specified index.
func insert(a []chan *printStatus, index int, value chan *printStatus) []chan *printStatus {
	if len(a) == index { // nil or empty slice or after last element
		return append(a, value)
	}
	a = append(a[:index+1], a[index:]...) // index < len(a)
	a[index] = value
	return a
}

// fanIn merges multiple channels into one.
func fanIn(ctx context.Context, channels []chan *printStatus) chan *printStatus {
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
}

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
