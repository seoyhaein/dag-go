package dag_go

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/seoyhaein/utils"
	"golang.org/x/sync/errgroup"
)

// TODO context 확인해야 함.

// ==================== 상수 정의 ====================

// Create, Exist, Fault are the result codes returned by createEdge.
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

// StartNode and EndNode are the reserved IDs for the synthetic entry and exit nodes
// that are automatically created by StartDag and FinishDag respectively.
const (
	StartNode = "start_node"
	EndNode   = "end_node"
)

// It is the node ID when the condition that the node cannot be created.
const noNodeID = "-1"

// ==================== 타입 정의 ====================

// DagConfig holds tunable parameters for a Dag instance.
// Use DefaultDagConfig for production-ready defaults, or override individual
// fields before passing to NewDagWithConfig.
type DagConfig struct {
	// MinChannelBuffer is the buffer size for inter-node edge channels.  Larger
	// values reduce the chance of a parent blocking while writing to a slow child.
	// Default: 5.
	MinChannelBuffer int

	// MaxChannelBuffer is the buffer capacity for the NodesResult and Errors
	// aggregation channels.  Set this higher than the total number of nodes to
	// prevent back-pressure stalls.  Default: 100.
	MaxChannelBuffer int

	// StatusBuffer is reserved for future per-node status channel buffering.
	// Default: 10.
	StatusBuffer int

	// WorkerPoolSize caps the number of goroutines that execute nodes concurrently.
	// If the number of nodes is smaller than WorkerPoolSize, the pool is sized to
	// the node count instead (min of the two).  Default: 50.
	WorkerPoolSize int

	// DefaultTimeout is the preFlight wait deadline applied to every node that
	// does not set its own Node.Timeout.  Zero means no extra timeout is added
	// beyond the caller's context deadline.  Default: 30 s.
	DefaultTimeout time.Duration

	// ErrorDrainTimeout is the maximum time collectErrors will wait to drain the
	// Errors channel.  Defaults to 5 s when left at zero (see DefaultDagConfig).
	ErrorDrainTimeout time.Duration

	// ExpectedNodeCount is a capacity hint for the internal nodes map and the
	// Edges slice.  When the final node count is known upfront, setting this
	// field avoids incremental map rehashing and slice growth during AddEdge
	// calls.  Zero means let the runtime decide the initial capacity.
	ExpectedNodeCount int
}

// DagOption is a functional-option type for NewDagWithOptions.
// Use the provided With* constructors to build option values.
type DagOption func(*Dag)

// nodeTask bundles the execution arguments for a single node run.
// Passing a concrete struct to the worker queue eliminates the closure
// allocation that the previous func() approach incurred per submission —
// removing per-node heap pressure in large DAG executions.
type nodeTask struct {
	node *Node
	sc   *SafeChannel[*printStatus]
	ctx  context.Context //nolint:containedctx // transient task descriptor; ctx is consumed immediately by the worker
}

// DagWorkerPool manages a bounded pool of goroutines for concurrent node execution.
// Tasks are submitted via Submit; Close drains the queue and waits for all workers.
type DagWorkerPool struct {
	workerLimit int
	taskQueue   chan nodeTask
	wg          sync.WaitGroup
	closeOnce   sync.Once // prevents double-close panic on taskQueue
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

// Dag is a Directed Acyclic Graph execution engine.
//
// A Dag is created with InitDag (or NewDag / NewDagWithConfig), populated with
// AddEdge calls, sealed with FinishDag, and then executed via the lifecycle:
//
//	ConnectRunner → GetReady → Start → Wait
//
// A completed DAG can be re-executed by calling Reset followed by the same
// lifecycle.  Dag must always be handled as a pointer; value-copy is forbidden
// because it embeds sync.RWMutex.
type Dag struct {
	// ID is the unique identifier assigned at creation time (UUID v4).
	ID string

	// Edges is the ordered list of directed edges in the graph.
	// Mutate only through AddEdge / AddEdgeIfNodesExist before FinishDag.
	Edges []*Edge

	nodes     map[string]*Node
	StartNode *Node // synthetic entry node created by StartDag
	EndNode   *Node // synthetic exit node created by FinishDag

	validated bool // true after FinishDag succeeds

	// NodesResult is the fan-in channel that collects printStatus events from
	// every node during execution.  Wait reads from this channel.
	NodesResult *SafeChannel[*printStatus]
	nodeResult  []*SafeChannel[*printStatus] // per-node result channels

	// errLogs collects structured errors for post-mortem inspection.
	errLogs []*systemError

	// Errors is the concurrency-safe error channel for runtime RunE failures.
	// Use reportError to write and collectErrors (or Errors.GetChannel()) to read.
	// The channel is recreated by Reset so it is valid for the next run.
	Errors *SafeChannel[error]

	// Timeout is the DAG-level execution deadline applied when bTimeout is true.
	// Set via WithTimeout or by assigning directly before GetReady.
	Timeout  time.Duration
	bTimeout bool

	// ContainerCmd is the global default Runnable applied to every node that
	// has no per-node override and no resolver match.
	// Set via SetContainerCmd to ensure thread-safe mutation.
	ContainerCmd Runnable

	runnerResolver RunnerResolver // optional dynamic runner selector

	// Config holds the tunable parameters active for this DAG instance.
	Config DagConfig

	workerPool     *DagWorkerPool
	nodeCount      int64        // total user-defined node count (atomic)
	completedCount int64        // nodes that called MarkCompleted (atomic)
	droppedErrors  int64        // errors dropped by reportError (atomic)
	mu             sync.RWMutex // guards nodes map, ContainerCmd, runnerResolver
}

// Edge represents a directed connection between two nodes.
// The embedded safeVertex channel carries runningStatus signals from the parent
// node to its child during execution.  Edges are created by AddEdge and reset
// by Reset; callers should not manipulate the fields directly.
type Edge struct {
	parentID   string
	childID    string
	safeVertex *SafeChannel[runningStatus]
}

// ==================== DAG 기본 및 옵션 함수 ====================

// DefaultDagConfig returns a DagConfig populated with production-ready defaults:
//   - MinChannelBuffer: 5
//   - MaxChannelBuffer: 100
//   - StatusBuffer:     10
//   - WorkerPoolSize:   50
//   - DefaultTimeout:   30 s
//   - ErrorDrainTimeout: 5 s
func DefaultDagConfig() DagConfig {
	return DagConfig{
		MinChannelBuffer:  5,
		MaxChannelBuffer:  100,
		StatusBuffer:      10,
		WorkerPoolSize:    50,
		DefaultTimeout:    30 * time.Second,
		ErrorDrainTimeout: 5 * time.Second,
	}
}

// NewDag returns a new Dag with default configuration (see DefaultDagConfig).
// Call StartDag (or use InitDag) to add the synthetic start node before adding edges.
func NewDag() *Dag {
	return NewDagWithConfig(DefaultDagConfig())
}

// NewDagWithConfig returns a new Dag using the supplied DagConfig.
// Prefer InitDag for the common case; use this constructor when you need to
// customise buffer sizes, timeouts, or the worker pool size before adding nodes.
func NewDagWithConfig(config DagConfig) *Dag {
	nodeCapacity := config.ExpectedNodeCount
	return &Dag{
		nodes:       make(map[string]*Node, nodeCapacity),
		Edges:       make([]*Edge, 0, nodeCapacity),
		ID:          uuid.NewString(),
		NodesResult: NewSafeChannelGen[*printStatus](config.MaxChannelBuffer),
		Config:      config,
		Errors:      NewSafeChannelGen[error](config.MaxChannelBuffer),
	}
}

// NewDagWithOptions returns a new Dag with DefaultDagConfig, then applies each
// DagOption in order.  Functional options (e.g. WithTimeout, WithWorkerPool)
// are applied after default values, so later options can override earlier ones.
func NewDagWithOptions(options ...DagOption) *Dag {
	dag := NewDagWithConfig(DefaultDagConfig())

	// 옵션 적용
	for _, option := range options {
		option(dag)
	}

	return dag
}

// InitDag creates a new DAG with default configuration and immediately calls
// StartDag to create the synthetic start node.  It is the recommended entry
// point for most users.
func InitDag() (*Dag, error) {
	dag := NewDag()
	if dag == nil {
		return nil, fmt.Errorf("failed to run NewDag")
	}
	return dag.StartDag()
}

// ==================== 전역 러너/리졸버 ====================

// SetContainerCmd sets the global default Runnable for all nodes in this DAG.
// It is safe to call concurrently.  Per-node overrides (SetNodeRunner) and the
// RunnerResolver take priority over this value; see runner priority in the README.
func (dag *Dag) SetContainerCmd(r Runnable) {
	// dag.ContainerCmd = r

	// 수정
	dag.mu.Lock()
	dag.ContainerCmd = r
	dag.mu.Unlock()

}

// loadDefaultRunnerAtomic 추가
/* func (dag *Dag) loadDefaultRunnerAtomic() Runnable {
	v := dag.defVal.Load()
	if v == nil {
		return nil
	}
	return v.(*runnerSlot).r
} */

// SetRunnerResolver installs a dynamic runner selector for this DAG.
// The resolver is called at execution time for each node, after the per-node
// atomic override is checked but before the global ContainerCmd fallback.
// Pass nil to clear a previously installed resolver.  Thread-safe.
func (dag *Dag) SetRunnerResolver(rr RunnerResolver) {
	// dag.runnerResolver = rr

	// 수정
	dag.mu.Lock()
	dag.runnerResolver = rr
	dag.mu.Unlock()
}

// 원자적으로 Resolver 반환
/* func (dag *Dag) loadRunnerResolverAtomic() RunnerResolver {
	// rrVal은 단지 "초기화 여부"를 위한 타입 고정용이고,
	// 실제 rr는 락으로 보호된 dag.runnerResolver에서 읽는다.
	// TODO 완전히 락-프리로 하려면 rrVal에 rr 자체를 담는 별도 래퍼 타입을 써야 함.
	dag.mu.RLock()
	rr := dag.runnerResolver
	dag.mu.RUnlock()
	return rr
} */

// SetNodeRunner sets the runner for the node with the given id.
// Returns false if the node does not exist or is not in Pending status.
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
		// We already hold n.mu — call runnerStore directly to avoid re-entrant lock.
		// (SetRunner would try to acquire n.mu again, causing a self-deadlock.)
		n.runnerStore(r)
		return true

	case NodeStatusRunning, NodeStatusSucceeded, NodeStatusFailed, NodeStatusSkipped:
		Log.Infof("SetNodeRunner ignored: node %s status=%v", n.ID, n.status)
		return false

	default:
		Log.Warnf("SetNodeRunner unknown status: node %s status=%v", n.ID, n.status)
		return false
	}
}

// SetNodeRunners bulk-sets runners from a map of node-id to Runnable.
// Returns the count of applied runners, and slices of missing/skipped node ids.
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
			// atomic.Value must always receive *runnerSlot to prevent a type-change panic.
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
	return applied, missing, skipped
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

// NewDagWorkerPool creates a new worker pool with the given number of goroutines.
// The internal task queue is buffered to twice the worker count so that
// callers are not serialised behind goroutine startup latency.
func NewDagWorkerPool(limit int) *DagWorkerPool {
	pool := &DagWorkerPool{
		workerLimit: limit,
		taskQueue:   make(chan nodeTask, limit*2),
	}

	for i := 0; i < limit; i++ { //nolint:intrange
		pool.wg.Add(1)
		go func() {
			defer pool.wg.Done()
			for task := range pool.taskQueue {
				// Skip execution if the caller's context is already done.
				select {
				case <-task.ctx.Done():
				default:
					task.node.runner(task.ctx, task.sc)
				}
			}
		}()
	}

	return pool
}

// Submit enqueues a nodeTask for execution by the worker pool.
func (p *DagWorkerPool) Submit(task nodeTask) {
	p.taskQueue <- task
}

// Close 워커 풀을 종료.
// sync.Once 를 통해 taskQueue 를 한 번만 닫으므로 이중 호출 시 패닉이 발생하지 않는다.
func (p *DagWorkerPool) Close() {
	p.closeOnce.Do(func() { close(p.taskQueue) })
	p.wg.Wait()
}

// ==================== Dag 메서드 ====================

// StartDag creates the synthetic start node and its trigger channel.
// It is called automatically by InitDag; call it directly only when building a
// DAG with NewDag or NewDagWithConfig.
//
// Returns the receiver so calls can be chained, or an error if the start node
// could not be created (e.g. the DAG is nil or the node was already created).
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

// reportError delivers err to the error channel in a non-blocking fashion.
// When the channel is full or closed, the error is not silently discarded:
// the droppedErrors counter is incremented (observable via DroppedErrors) and
// a structured log entry is emitted so that log-aggregation pipelines can alert
// on the event.  The cumulative drop count is included in the log field
// "dropped_total" to make SLO alerting straightforward.
func (dag *Dag) reportError(err error) {
	if !dag.Errors.Send(err) {
		n := atomic.AddInt64(&dag.droppedErrors, 1)
		Log.WithField("dag_id", dag.ID).
			WithField("dropped_total", n).
			WithError(err).
			Warn("error channel full or closed; dropping error")
	}
}

// collectErrors drains the error channel until it is empty, or until
// DagConfig.ErrorDrainTimeout (default 5 s) or ctx fires — whichever is first.
func (dag *Dag) collectErrors(ctx context.Context) []error {
	var errs []error

	drainTimeout := dag.Config.ErrorDrainTimeout
	if drainTimeout <= 0 {
		//nolint:mnd // 5 s safe fallback when ErrorDrainTimeout was not set in DagConfig.
		drainTimeout = 5 * time.Second
	}
	timeout := time.After(drainTimeout)
	ch := dag.Errors.GetChannel()

	for {
		select {
		case err := <-ch:
			errs = append(errs, err)
		case <-timeout:
			return errs
		case <-ctx.Done():
			return errs
		default:
			if len(errs) > 0 {
				// Wait briefly for any in-flight errors before declaring done.
				select {
				case err := <-ch:
					errs = append(errs, err)
				case <-time.After(100 * time.Millisecond): //nolint:mnd
					return errs
				}
			} else {
				//nolint:mnd // short poll sleep; TODO: replace with proper drain helper.
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
//
//nolint:gocognit // iterates over three independent channel slices; each branch is simple but overall count is high
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

	if dag.Errors != nil {
		if err := dag.Errors.Close(); err != nil {
			Log.Warnf("Failed to close Errors channel: %v", err)
		} else {
			Log.Info("Closed Errors channel")
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

// Progress returns the DAG execution completion ratio in [0.0, 1.0].
//
// NOTE: nodeCount and completedCount are read in two separate atomic operations;
// they do not form an atomic pair.  completedCount may be incremented between the
// two reads, making the returned ratio momentarily slightly ahead of reality.
// This is acceptable for progress-bar or observability purposes, but must NOT
// be used for correctness decisions (e.g. deciding whether all nodes finished).
func (dag *Dag) Progress() float64 {
	nodeCount := atomic.LoadInt64(&dag.nodeCount)
	if nodeCount == 0 {
		return 0.0
	}
	completedCount := atomic.LoadInt64(&dag.completedCount)
	return float64(completedCount) / float64(nodeCount)
}

// DroppedErrors returns the number of errors that reportError could not deliver
// to the Errors channel (channel full or closed) since the DAG was created or
// last Reset.  A non-zero value indicates that DagConfig.MaxChannelBuffer is
// too small or that the Errors channel consumer is not draining fast enough.
func (dag *Dag) DroppedErrors() int64 {
	return atomic.LoadInt64(&dag.droppedErrors)
}

// CreateNode creates a pointer to a new node with thread safety.
func (dag *Dag) CreateNode(id string) *Node {
	dag.mu.Lock()
	defer dag.mu.Unlock()

	return dag.createNode(id)
}

// CreateNodeWithTimeOut creates a node that applies a per-node timeout when bTimeOut is true.
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
	// node.runnerStore(dag.ContainerCmd)
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

	// 엣지 생성 및 검증
	edge, check := dag.createEdge(fromNode.ID, toNode.ID)
	if check == Fault || check == Exist {
		return logErr(fmt.Errorf("edge cannot be created"))
	}
	if edge == nil {
		return logErr(fmt.Errorf("vertex is nil"))
	}

	// 자식과 부모 관계 설정은 엣지 생성이 성공한 후에 수행
	fromNode.children = append(fromNode.children, toNode)
	toNode.parent = append(toNode.parent, fromNode)

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

	// 엣지 생성 및 검증
	edge, check := dag.createEdge(fromNode.ID, toNode.ID)
	if check == Fault || check == Exist {
		return logErr(fmt.Errorf("edge cannot be created"))
	}
	if edge == nil {
		return logErr(fmt.Errorf("vertex is nil"))
	}

	// 자식과 부모 관계 설정은 엣지 생성이 성공한 후에 수행
	fromNode.children = append(fromNode.children, toNode)
	toNode.parent = append(toNode.parent, fromNode)

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

	// 엣지 생성 및 체크
	edge, check := dag.createEdge(fromNode.ID, toNode.ID)
	if check == Fault || check == Exist {
		return logErr(fmt.Errorf("edge cannot be created"))
	}
	if edge == nil {
		return logErr(fmt.Errorf("vertex is nil"))
	}

	// 부모-자식 관계 설정은 엣지 생성이 성공한 후에 수행
	fromNode.children = append(fromNode.children, toNode)
	toNode.parent = append(toNode.parent, fromNode)

	// 엣지의 vertex를 양쪽 노드에 추가
	fromNode.childrenVertex = append(fromNode.childrenVertex, edge.safeVertex)
	toNode.parentVertex = append(toNode.parentVertex, edge.safeVertex)

	return nil
}

// FinishDag finalizes the DAG by connecting end nodes and validating the structure.
//
//nolint:gocognit // DAG finalization requires multiple sequential validation steps; splitting would obscure the overall flow
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

	// 사이클 검사: FinishDag 는 이미 dag.mu.Lock() 을 보유하고 있으므로
	// RLock 을 추가로 획득하는 DetectCycle 대신 내부 함수 detectCycle 을 직접 호출.
	if detectCycle(dag) {
		return logErr(fmt.Errorf("FinishDag: %w", ErrCycleDetected))
	}

	// 검증 완료 플래그 설정
	dag.validated = true
	return nil
}

// Reset reinitialises a completed DAG so it can be executed again without
// rebuilding the graph from scratch.
//
// Reset MUST be called only after Wait returns.  Calling it while the DAG is
// still running leads to undefined behaviour because Reset replaces channels
// that active goroutines may be reading from or writing to.
//
// After Reset, follow the standard execution lifecycle:
//
//	dag.Reset()
//	dag.ConnectRunner()
//	dag.GetReady(ctx)
//	dag.Start()
//	dag.Wait(ctx)
//
// The graph topology (nodes, edges, Config, ContainerCmd, runners) is preserved;
// only execution state is reset.
func (dag *Dag) Reset() {
	dag.mu.Lock()
	defer dag.mu.Unlock()

	// Reset per-run counters to zero.
	atomic.StoreInt64(&dag.completedCount, 0)
	atomic.StoreInt64(&dag.droppedErrors, 0)

	// Reset every node to Pending and clear its per-execution state.
	// childrenVertex / parentVertex are cleared here and rebuilt from dag.Edges
	// below so they point to the newly created SafeChannels.
	for _, n := range dag.nodes {
		n.mu.Lock()
		n.status = NodeStatusPending
		n.succeed = false
		n.runner = nil
		n.childrenVertex = nil
		n.parentVertex = nil
		n.mu.Unlock()
	}

	// Re-attach the start-node trigger channel.  This channel is not backed by
	// an Edge entry; Start() writes to parentVertex[0] to fire the first node.
	if dag.StartNode != nil {
		dag.StartNode.mu.Lock()
		dag.StartNode.parentVertex = []*SafeChannel[runningStatus]{
			NewSafeChannelGen[runningStatus](dag.Config.MinChannelBuffer),
		}
		dag.StartNode.mu.Unlock()
	}

	// Recreate each edge's channel and rewire it into the owning nodes' vertex
	// slices.  The lock order (dag.mu → n.mu) is consistent with the rest of
	// the codebase and prevents deadlocks.
	for _, edge := range dag.Edges {
		edge.safeVertex = NewSafeChannelGen[runningStatus](dag.Config.MinChannelBuffer)
		if parent := dag.nodes[edge.parentID]; parent != nil {
			parent.mu.Lock()
			parent.childrenVertex = append(parent.childrenVertex, edge.safeVertex)
			parent.mu.Unlock()
		}
		if child := dag.nodes[edge.childID]; child != nil {
			child.mu.Lock()
			child.parentVertex = append(child.parentVertex, edge.safeVertex)
			child.mu.Unlock()
		}
	}

	// Recreate the aggregated result and error channels that Wait → closeChannels
	// closed at the end of the previous run.
	dag.NodesResult = NewSafeChannelGen[*printStatus](dag.Config.MaxChannelBuffer)
	dag.Errors = NewSafeChannelGen[error](dag.Config.MaxChannelBuffer)

	// Clear per-run state so GetReady and the worker pool are re-initialised
	// fresh on the next call.
	dag.nodeResult = nil
	dag.workerPool = nil
	dag.errLogs = nil
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

// ConnectRunner attaches a runner closure (the three-phase preFlight / inFlight /
// postFlight state machine) to every node.
//
// Call ConnectRunner after FinishDag and before GetReady.  After Reset, call it
// again so the closures reference the freshly created channels.
//
// Returns false if the DAG has no nodes.
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

// GetReady initialises the worker pool and submits a goroutine task for every
// node.  The worker pool size is min(nodeCount, DagConfig.WorkerPoolSize).
//
// Call GetReady after ConnectRunner and before Start.  ctx is forwarded to each
// node's runner function; cancel it to abort the entire execution.
//
// Returns false if the DAG has no nodes or GetReady has already been called
// (i.e. nodeResult is already set).
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
	safeChs := make([]*SafeChannel[*printStatus], 0, n)

	for _, v := range dag.nodes {
		// Per-node result channel: collects the three phase statuses
		// (preFlight / inFlight / postFlight) that fanIn forwards to NodesResult.
		sc := NewSafeChannelGen[*printStatus](dag.Config.MinChannelBuffer)
		safeChs = append(safeChs, sc)

		// Submit a zero-allocation nodeTask instead of a closure so the worker
		// goroutine calls node.runner directly without a heap-allocated func().
		dag.workerPool.Submit(nodeTask{node: v, sc: sc, ctx: ctx})
	}

	if dag.nodeResult != nil {
		return false
	}
	dag.nodeResult = safeChs
	return true
}

// Start fires the DAG by sending a trigger signal to the start node's input
// channel.  All node goroutines are already waiting (submitted by GetReady);
// this single send unblocks the start node and cascades through the graph.
//
// Call Start exactly once after GetReady.  Returns false if the trigger send
// fails (e.g. channel full or closed, which should not happen in normal use).
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

// Wait blocks until the DAG finishes execution, ctx expires, or a fatal node
// failure is detected on NodesResult.
//
// It returns true only when the end node emits a FlightEnd status — meaning
// every node in the graph reached a terminal state (Succeeded or Skipped).
// It returns false on any of:
//   - context cancellation or timeout
//   - NodesResult channel closed unexpectedly
//   - end node reporting a PreflightFailed / InFlightFailed / PostFlightFailed
//
// Wait closes all channels (closeChannels) and shuts down the worker pool when
// it returns, regardless of whether execution succeeded.  Do NOT use the DAG
// after Wait returns without first calling Reset.
//
//nolint:gocognit // fan-in select loop must handle merge result, node status stream, and context cancellation simultaneously
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

	// mergeResult is a one-shot buffered channel: the goroutine writes exactly
	// once and then exits, so no close is ever needed.  The buffer of 1 prevents
	// the goroutine from blocking if Wait returns early (e.g. on ctx.Done).
	mergeResult := make(chan bool, 1)
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
				// Save fields before returning c to the pool, because
				// releasePrintStatus zeroes the struct fields.
				rStatus := c.rStatus
				releasePrintStatus(c)
				if rStatus == PreflightFailed ||
					rStatus == InFlightFailed ||
					rStatus == PostFlightFailed {
					return false
				}
				if rStatus == FlightEnd {
					return true
				}
			} else {
				releasePrintStatus(c)
			}
		case <-waitCtx.Done():
			Log.Printf("DAG execution timed out or canceled: %v", waitCtx.Err())
			return false
		}
	}
}

// dfsState holds the per-traversal book-keeping maps used by detectCycleDFS.
// Both maps are reused across calls via dfsStatePool to avoid per-call heap
// allocations; clear() resets entries without freeing the backing memory.
type dfsState struct {
	visited  map[string]bool
	recStack map[string]bool
}

// dfsStatePool provides reusable dfsState objects for detectCycle.
var dfsStatePool = sync.Pool{
	New: func() any {
		return &dfsState{
			visited:  make(map[string]bool),
			recStack: make(map[string]bool),
		}
	},
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

// detectCycle checks if the DAG contains a cycle without acquiring a lock.
// The caller must guarantee that dag.nodes is not mutated during this call
// (e.g. by holding dag.mu in any mode).  FinishDag calls this directly because
// it already holds dag.mu.Lock(); the exported DetectCycle acquires RLock first.
func detectCycle(dag *Dag) bool {
	// Acquire a pooled dfsState and reset its maps without freeing backing
	// memory (clear is O(N) but allocation-free, unlike make).
	state := dfsStatePool.Get().(*dfsState)
	clear(state.visited)
	clear(state.recStack)
	defer dfsStatePool.Put(state)

	// Traverse dag.nodes directly — no copyDag needed because the caller
	// guarantees dag.mu is held (FinishDag holds Lock; DetectCycle holds RLock).
	for _, node := range dag.nodes {
		if !state.visited[node.ID] {
			if detectCycleDFS(node, state.visited, state.recStack) {
				return true
			}
		}
	}
	return false
}

// DetectCycle checks if the DAG contains a directed cycle.  It is safe to call
// concurrently: it acquires a read lock before inspecting the graph.
//
// Internal callers that already hold dag.mu (e.g. FinishDag) must call
// detectCycle directly to avoid a re-entrant lock attempt.
func DetectCycle(dag *Dag) bool {
	dag.mu.RLock()
	defer dag.mu.RUnlock()
	return detectCycle(dag)
}

// connectRunner connects a runner function to a node.
//
//nolint:gocognit // three-phase flight state machine with CAS guards; splitting would obscure the node lifecycle
func connectRunner(n *Node) {
	n.runner = func(ctx context.Context, result *SafeChannel[*printStatus]) {
		// sendResult delivers a copy of ps to the per-node result channel.
		// SendBlocking ensures the monitoring event is never silently dropped:
		// if the result buffer is momentarily full the send waits for a consumer.
		// If ctx is cancelled the pool-acquired copy is returned to prevent leak.
		sendResult := func(ps *printStatus) {
			copied := newPrintStatus(ps.rStatus, ps.nodeID)
			if !result.SendBlocking(ctx, copied) {
				releasePrintStatus(copied)
			}
		}

		// CheckParentsStatus internally calls TransitionStatus(Pending, Skipped)
		// when a parent has failed, so no additional SetStatus call is needed here.
		if !n.CheckParentsStatus() {
			ps := newPrintStatus(PostFlightFailed, n.ID)
			sendResult(ps)
			n.notifyChildren(ctx, Failed)
			releasePrintStatus(ps)
			return
		}

		// Pending → Running: guarded CAS — rejects the transition if the node is
		// not Pending (e.g. already Skipped by a concurrent parent failure).
		if ok := n.TransitionStatus(NodeStatusPending, NodeStatusRunning); !ok {
			Log.Warnf("connectRunner: Pending→Running rejected for node %s (status=%v)", n.ID, n.GetStatus())
		}

		// preFlight phase
		ps := preFlight(ctx, n)
		sendResult(ps)
		if ps.rStatus == PreflightFailed {
			if ok := n.TransitionStatus(NodeStatusRunning, NodeStatusFailed); !ok {
				Log.Warnf("connectRunner: Running→Failed rejected for node %s (status=%v)", n.ID, n.GetStatus())
			}
			n.notifyChildren(ctx, Failed)
			releasePrintStatus(ps)
			return
		}
		releasePrintStatus(ps)

		// inFlight phase
		ps = inFlight(ctx, n)
		sendResult(ps)
		if ps.rStatus == InFlightFailed {
			if ok := n.TransitionStatus(NodeStatusRunning, NodeStatusFailed); !ok {
				Log.Warnf("connectRunner: Running→Failed rejected for node %s (status=%v)", n.ID, n.GetStatus())
			}
			n.notifyChildren(ctx, Failed)
			releasePrintStatus(ps)
			return
		}
		releasePrintStatus(ps)

		// postFlight phase — ctx forwarded so SendBlocking can abort on cancel.
		ps = postFlight(ctx, n)
		sendResult(ps)
		if ps.rStatus == PostFlightFailed {
			if ok := n.TransitionStatus(NodeStatusRunning, NodeStatusFailed); !ok {
				Log.Warnf("connectRunner: Running→Failed rejected for node %s (status=%v)", n.ID, n.GetStatus())
			}
		} else {
			if ok := n.TransitionStatus(NodeStatusRunning, NodeStatusSucceeded); !ok {
				Log.Warnf("connectRunner: Running→Succeeded rejected for node %s (status=%v)", n.ID, n.GetStatus())
			}
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

// fanIn merges multiple channels into one using errgroup for structured cancellation.
//
// No concurrency limit is applied: the relay goroutines are I/O-bound (blocked
// on channel reads) and must all run concurrently.  Adding errgroup.SetLimit
// would serialize completions and increase tail latency without bounding CPU.
func fanIn(ctx context.Context, channels []*SafeChannel[*printStatus], merged *SafeChannel[*printStatus]) bool {
	eg, egCtx := errgroup.WithContext(ctx)

	for _, sc := range channels {
		eg.Go(func() error {
			for val := range sc.GetChannel() {
				if !merged.SendBlocking(egCtx, val) {
					return egCtx.Err()
				}
			}
			return nil
		})
	}

	return eg.Wait() == nil
}

// min returns the minimum of two integers.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ToMermaid generates a Mermaid flowchart string that represents the DAG topology.
//
// The output uses the "graph TD" (top-down) direction.  Synthetic nodes
// (start_node / end_node) are rendered with a stadium shape to distinguish
// infrastructure nodes from user-defined ones.  If a per-node Runnable has been
// registered via SetNodeRunner, its concrete Go type is appended to the node
// label after a line-break so the diagram shows which executor is bound to each
// step — useful for debugging or documentation.
//
// Node IDs are sanitised for Mermaid syntax by replacing any character that is
// not an ASCII letter, digit, or underscore with an underscore (see
// mermaidSafeID).  This prevents parser errors for IDs that contain hyphens,
// dots, or spaces.
//
// Example output:
//
//	graph TD
//	    start_node(["start_node"])
//	    A["A\n*main.MyRunner"]
//	    B["B"]
//	    end_node(["end_node"])
//	    start_node --> A
//	    A --> B
//	    B --> end_node
//
// ToMermaid acquires a read-lock and is safe to call concurrently with
// Progress() and other read-only observers.  It must be called after
// FinishDag so that dag.Edges is complete; calling it before FinishDag will
// produce a diagram that is missing the edges to the synthetic end node.
func (dag *Dag) ToMermaid() string {
	dag.mu.RLock()
	defer dag.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("graph TD\n")

	// First pass — emit a labelled node definition for every node that appears
	// in at least one edge.  Pre-sizing the map to the number of known nodes
	// avoids rehash growth on graphs where every node participates in an edge.
	defined := make(map[string]bool, len(dag.nodes))
	for _, edge := range dag.Edges {
		dag.writeMermaidNode(&sb, edge.parentID, defined)
		dag.writeMermaidNode(&sb, edge.childID, defined)
	}

	// Second pass — emit directed edges.
	for _, edge := range dag.Edges {
		fmt.Fprintf(&sb, "    %s --> %s\n",
			mermaidSafeID(edge.parentID),
			mermaidSafeID(edge.childID))
	}

	return sb.String()
}

// writeMermaidNode emits a single Mermaid node definition into sb unless the
// node has already been defined (tracked via the defined map).
// Synthetic nodes (start_node / end_node) use the stadium shape; all others
// use the default rectangle.  Caller must hold dag.mu at least for reading.
func (dag *Dag) writeMermaidNode(sb *strings.Builder, nodeID string, defined map[string]bool) {
	if defined[nodeID] {
		return
	}
	defined[nodeID] = true
	mID := mermaidSafeID(nodeID)
	switch nodeID {
	case StartNode, EndNode:
		// Stadium shape visually distinguishes synthetic infrastructure nodes.
		fmt.Fprintf(sb, "    %s([\"%s\"])\n", mID, nodeID)
	default:
		fmt.Fprintf(sb, "    %s[\"%s\"]\n", mID, mermaidNodeLabel(nodeID, dag.nodes[nodeID]))
	}
}

// mermaidNodeLabel returns the display label for a Mermaid node box.
// When a per-node Runnable has been registered via SetNodeRunner, its concrete
// Go type is appended after a line-break as a diagnostic hint visible in the
// rendered diagram.
func mermaidNodeLabel(nodeID string, n *Node) string {
	if n != nil {
		if r := n.runnerLoad(); r != nil {
			return fmt.Sprintf("%s\\n%T", nodeID, r)
		}
	}
	return nodeID
}

// mermaidSafeID converts an arbitrary node ID to a Mermaid-safe identifier by
// replacing any character that is not an ASCII letter, digit, or underscore
// with an underscore.  This prevents syntax errors when node IDs contain
// hyphens, spaces, dots, or other characters that would break the flowchart
// parser.
func mermaidSafeID(id string) string {
	var b strings.Builder
	b.Grow(len(id))
	for _, c := range id {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			b.WriteRune(c)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

// CopyDag creates a structural copy of original with a new ID.
//
// The copy preserves the graph topology (nodes, edges, parent/child relationships)
// and all DagConfig values.  It owns independent channels (NodesResult, Errors,
// edge safeVertices) so executing the copy cannot affect the original.
//
// Items NOT copied: workerPool, nodeResult, errLogs, and runnerResolver.
// The caller must wire runners and call the standard lifecycle on the copy:
//
//	ConnectRunner → GetReady → Start → Wait
//
// Returns nil if original is nil or newID is empty.
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
		Config:       original.Config, // copy all DagConfig fields
		NodesResult:  NewSafeChannelGen[*printStatus](original.Config.MaxChannelBuffer),
		Errors:       NewSafeChannelGen[error](original.Config.MaxChannelBuffer),
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
			ID: n.ID, // Node 구조체가 Id로 되어 있다면 그대로 유지
			// 기타 필드는 cycle 검증에 필요하지 않으므로 생략
		}
		newNodes[newNode.ID] = newNode
	}

	// 2. 원본 노드의 부모/자식 관계를 이용하여 새 노드들의 포인터 연결
	for _, n := range original.nodes {
		newNode := newNodes[n.ID]
		// Pre-allocate slices to the exact capacity needed so each append
		// below copies at most one node pointer without triggering a grow.
		newNode.parent = make([]*Node, 0, len(n.parent))
		newNode.children = make([]*Node, 0, len(n.children))
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
			parentID: e.parentID,
			childID:  e.childID,
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
