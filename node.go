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

// NodeStatus represents the lifecycle state of a Node.
type NodeStatus int

// NodeStatusPending through NodeStatusSkipped represent the lifecycle states of a Node.
const (
	NodeStatusPending NodeStatus = iota
	NodeStatusRunning
	NodeStatusSucceeded
	NodeStatusFailed
	NodeStatusSkipped // set when a parent has failed
)

// Node is the fundamental building block of a DAG.
// A Node must always be handled as a pointer; copying a Node is forbidden
// because atomic.Value must not be copied after first use.
type Node struct {
	ID        string
	ImageName string

	// runnerVal holds the per-node Runnable wrapped in *runnerSlot.
	// Rules:
	//   - Store exclusively via runnerStore() — never assign directly.
	//   - Load exclusively via runnerLoad() / getRunnerSnapshot().
	//   - Node must be passed as a pointer; value-copy is forbidden.
	//   - First Store must be non-nil (*runnerSlot wrapper is always non-nil;
	//     the inner Runnable field may be nil when no runner is set yet).
	runnerVal atomic.Value // stores *runnerSlot

	children  []*Node // child node list
	parent    []*Node // parent node list
	parentDag *Dag    // owning DAG
	Commands  string

	// childrenVertex / parentVertex use SafeChannel to prevent double-close panics.
	childrenVertex []*SafeChannel[runningStatus]
	parentVertex   []*SafeChannel[runningStatus]
	runner         func(ctx context.Context, result *SafeChannel[*printStatus])

	// synchronisation fields
	status  NodeStatus
	succeed bool
	mu      sync.RWMutex // guards status and succeed

	// per-node timeout configuration
	Timeout  time.Duration // effective only when bTimeout is true
	bTimeout bool          // true → apply Timeout during preFlight
}

// SetStatus sets the node's status under the write lock.
// Prefer TransitionStatus when a pre-condition on the current status is required.
func (n *Node) SetStatus(status NodeStatus) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.status = status
}

// GetStatus returns the node's current status under the read lock.
func (n *Node) GetStatus() NodeStatus {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.status
}

// isValidTransition reports whether the from→to NodeStatus edge exists in the
// Node state machine.
//
// Valid transitions:
//
//	Pending  → Running | Skipped
//	Running  → Succeeded | Failed
//	Succeeded, Failed, Skipped → (terminal; no outgoing transitions)
func isValidTransition(from, to NodeStatus) bool {
	switch from {
	case NodeStatusPending:
		return to == NodeStatusRunning || to == NodeStatusSkipped
	case NodeStatusRunning:
		return to == NodeStatusSucceeded || to == NodeStatusFailed
	default:
		// Terminal states have no outgoing transitions.
		return false
	}
}

// TransitionStatus atomically advances n's status from `from` to `to`.
// It returns true only when both conditions hold:
//  1. n.status == from at the moment the lock is acquired, AND
//  2. the from→to transition is permitted by the state machine.
//
// This prevents illegal backwards moves such as Failed→Succeeded.
// Use SetStatus only when an unconditional override is explicitly required.
func (n *Node) TransitionStatus(from, to NodeStatus) bool {
	if !isValidTransition(from, to) {
		return false
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.status != from {
		return false
	}
	n.status = to
	return true
}

// SetSucceed sets the succeed flag under the write lock.
func (n *Node) SetSucceed(val bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.succeed = val
}

// IsSucceed returns the succeed flag under the read lock.
func (n *Node) IsSucceed() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.succeed
}

// CheckParentsStatus returns false (and transitions this node to Skipped) if any
// parent has already failed.  The Pending→Skipped transition is guarded by
// TransitionStatus so that a node in an unexpected state is never silently
// overwritten.
func (n *Node) CheckParentsStatus() bool {
	for _, parent := range n.parent {
		if parent.GetStatus() == NodeStatusFailed {
			n.TransitionStatus(NodeStatusPending, NodeStatusSkipped)
			return false
		}
	}
	return true
}

// MarkCompleted increments the parent DAG's completed-node counter.
func (n *Node) MarkCompleted() {
	if n.parentDag != nil && n.ID != StartNode && n.ID != EndNode {
		atomic.AddInt64(&n.parentDag.completedCount, 1)
	}
}

// NodeError carries structured information about a node-level execution failure.
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

// preFlight waits for all parent channels to report a non-Failed status.
// The timeout applied follows this priority:
//  1. Node.Timeout   (when Node.bTimeout is true)
//  2. Dag.Config.DefaultTimeout (when positive)
//  3. No extra timeout — honour the caller's existing deadline.
//
//nolint:gocognit,gocyclo // fan-in select over multiple parent channels; complexity is inherent to the concurrent coordination logic.
func preFlight(ctx context.Context, n *Node) *printStatus {
	if n == nil {
		return newPrintStatus(PreflightFailed, noNodeID)
	}

	// Determine effective timeout for this preFlight call.
	var timeoutCtx context.Context
	var cancel context.CancelFunc

	switch {
	case n.bTimeout && n.Timeout > 0:
		// Per-node timeout takes highest priority.
		timeoutCtx, cancel = context.WithTimeout(ctx, n.Timeout)
	case n.parentDag != nil && n.parentDag.Config.DefaultTimeout > 0:
		// Fall back to the DAG-level default timeout.
		timeoutCtx, cancel = context.WithTimeout(ctx, n.parentDag.Config.DefaultTimeout)
	default:
		// No additional timeout; honour the caller's existing deadline / cancellation.
		timeoutCtx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	// Build an errgroup that limits concurrent goroutines waiting on parent channels.
	eg, egCtx := errgroup.WithContext(timeoutCtx)
	//nolint:mnd // 10 is the fixed limit for concurrent goroutines; TODO: make configurable.
	eg.SetLimit(10)

	i := len(n.parentVertex)

	for j := 0; j < i; j++ { //nolint:intrange
		k := j // capture loop variable
		sc := n.parentVertex[k]
		if sc == nil {
			Log.Fatalf("preFlight: n.parentVertex[%d] is nil for node %s", k, n.ID)
		}
		eg.Go(func() error {
			nodeID, chIdx := n.ID, k
			lbl := pprof.Labels(
				"phase", "preFlight",
				"nodeId", nodeID,
				"channelIndex", strconv.Itoa(chIdx),
			)
			pprof.SetGoroutineLabels(pprof.WithLabels(egCtx, lbl))

			// Debug breakpoints (compiled away in production via build tag).
			if nodeID == "C" {
				debugonly.BreakHere()
			}
			if nodeID == "node1" && chIdx == 2 {
				debugonly.BreakHere()
			}

			select {
			case result := <-sc.GetChannel():
				if result == Failed {
					return fmt.Errorf("node %s: parent channel returned Failed", n.ID)
				}
				return nil
			case <-egCtx.Done():
				return egCtx.Err()
			}
		})
	}

	err := eg.Wait()
	if err == nil {
		n.SetSucceed(true)
		Log.Println("Preflight", n.ID)
		return newPrintStatus(Preflight, n.ID)
	}

	if err != nil {
		nodeErr := &NodeError{NodeID: n.ID, Phase: "preflight", Err: err}
		Log.Println(nodeErr.Error())
	}

	n.SetSucceed(false)
	Log.Println("Preflight failed for node", n.ID, "error:", err)
	return newPrintStatus(PreflightFailed, n.ID)
}

// inFlight runs the node's Runnable via Execute.  ctx is forwarded so that
// cancellation / timeout signals reach the user-supplied RunE implementation.
func inFlight(ctx context.Context, n *Node) *printStatus {
	if n == nil {
		return newPrintStatus(InFlightFailed, noNodeID)
	}

	// StartNode and EndNode do not execute user code.
	if n.ID == StartNode || n.ID == EndNode {
		n.SetSucceed(true)
		Log.Println("InFlight (special node)", n.ID)
		return newPrintStatus(InFlight, n.ID)
	}

	// General node execution.
	// Note: status is already NodeStatusRunning, set by connectRunner via
	// TransitionStatus(Pending, Running) before this function is called.
	if n.IsSucceed() {
		if err := n.Execute(ctx); err != nil {
			n.SetSucceed(false)
			nodeErr := &NodeError{NodeID: n.ID, Phase: "inflight", Err: err}
			Log.Println(nodeErr.Error())
		}
	} else {
		Log.Println("Skipping execution for node", n.ID, "due to previous failure")
	}

	if n.IsSucceed() {
		Log.Println("InFlight", n.ID)
		return newPrintStatus(InFlight, n.ID)
	}
	Log.Println("InFlightFailed", n.ID)
	return newPrintStatus(InFlightFailed, n.ID)
}

// postFlight notifies all child channels and marks the node as completed.
// ctx is forwarded to SendBlocking so that a cancelled context unblocks the
// send rather than leaving a child goroutine waiting forever in preFlight.
func postFlight(ctx context.Context, n *Node) *printStatus {
	if n == nil {
		return newPrintStatus(PostFlightFailed, noNodeID)
	}

	if n.ID == EndNode {
		Log.Println("FlightEnd", n.ID)
		return newPrintStatus(FlightEnd, n.ID)
	}

	var result runningStatus
	if n.IsSucceed() {
		result = Succeed
	} else {
		result = Failed
	}

	// SendBlocking guarantees the signal reaches the child or returns false
	// when ctx is cancelled — eliminating the silent-drop deadlock risk.
	for _, sc := range n.childrenVertex {
		if !sc.SendBlocking(ctx, result) {
			Log.Warnf("postFlight: signal delivery to child of node %s interrupted: %v", n.ID, ctx.Err())
		}
	}

	n.MarkCompleted()

	Log.Println("PostFlight", n.ID)
	return newPrintStatus(PostFlight, n.ID)
}

// Execute runs the node's resolved Runnable, forwarding ctx for cancellation.
func (n *Node) Execute(ctx context.Context) error {
	return execute(ctx, n)
}

// createNode creates a Node pre-loaded with the given Runnable.
func createNode(id string, r Runnable) *Node {
	n := &Node{ID: id, status: NodeStatusPending}
	n.runnerStore(r) // first Store (non-nil *runnerSlot wrapper)
	return n
}

// createNodeWithID creates a Node with no runner pre-loaded.
func createNodeWithID(id string) *Node {
	n := &Node{ID: id, status: NodeStatusPending}
	n.runnerStore(nil) // first Store ensures atomic.Value is initialised
	return n
}

// notifyChildren delivers st to every child vertex channel using SendBlocking.
// ctx is used to abort the send when the execution context is cancelled so
// that a failed parent never leaves its children blocked in preFlight.
func (n *Node) notifyChildren(ctx context.Context, st runningStatus) {
	for _, sc := range n.childrenVertex {
		if !sc.SendBlocking(ctx, st) {
			Log.Warnf("notifyChildren: signal delivery from node %s interrupted: %v", n.ID, ctx.Err())
		}
	}
}

// checkVisit returns true when every entry in the map is true.
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

// getNode returns the Node with the given id from the map, or nil.
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

// getNextNode pops and returns the first child node of n.
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

// statusPool is a sync.Pool for printStatus objects to reduce allocations.
var statusPool = sync.Pool{
	New: func() interface{} {
		return &printStatus{}
	},
}

// newPrintStatus acquires a printStatus from the pool and initialises it.
func newPrintStatus(status runningStatus, nodeID string) *printStatus {
	ps := statusPool.Get().(*printStatus)
	ps.rStatus = status
	ps.nodeID = nodeID
	return ps
}

// releasePrintStatus resets and returns ps to the pool.
func releasePrintStatus(ps *printStatus) {
	ps.rStatus = 0
	ps.nodeID = ""
	statusPool.Put(ps)
}
