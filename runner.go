package dag_go

// for late binding

import (
	"context"
	"errors"
)

// ErrNoRunner is returned by execute when no Runnable has been configured for the node.
var ErrNoRunner = errors.New("no runner set for node")

// RunnerResolver is an optional hook that picks a Runnable dynamically based on
// node metadata (e.g. image name, labels).  It is consulted after the per-node
// atomic runner but before Dag.ContainerCmd.
type RunnerResolver func(*Node) Runnable

// runnerSlot is the fixed wrapper type stored inside Node.runnerVal (atomic.Value).
// Using a dedicated concrete type prevents the "inconsistent type" panic that
// occurs when different concrete types are stored across calls.
type runnerSlot struct{ r Runnable }

// runnerStore stores r inside n.runnerVal wrapped in a *runnerSlot.
// This must be the ONLY way values are written to runnerVal.
func (n *Node) runnerStore(r Runnable) {
	n.runnerVal.Store(&runnerSlot{r: r})
}

// runnerLoad returns the Runnable stored in n.runnerVal, or nil if none was set.
func (n *Node) runnerLoad() Runnable {
	v := n.runnerVal.Load()
	if v == nil {
		return nil
	}
	return v.(*runnerSlot).r
}

// getRunnerSnapshot resolves the effective Runnable for this node at call time.
// Priority: Node.runnerVal (atomic) > Dag.RunnerResolver > Dag.ContainerCmd.
func (n *Node) getRunnerSnapshot() Runnable {
	// 1. Per-node override stored atomically.
	if r := n.runnerLoad(); r != nil {
		return r
	}

	// 2. DAG-level resolver and fallback, protected by a read lock.
	var rr RunnerResolver
	var base Runnable
	if d := n.parentDag; d != nil {
		d.mu.RLock()
		rr = d.runnerResolver
		base = d.ContainerCmd
		d.mu.RUnlock()
	}
	if rr != nil {
		if r := rr(n); r != nil {
			return r
		}
	}
	return base
}

// SetRunner atomically sets the runner for a Pending node.
// Returns false if the node is not in Pending status.
func (n *Node) SetRunner(r Runnable) bool {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.status != NodeStatusPending {
		// Reject changes to nodes that are already running / done / skipped.
		return false
	}
	n.runnerStore(r)
	return true
}

// execute calls the node's resolved Runnable with the given context.
// Returns ErrNoRunner when no Runnable has been configured.
func execute(ctx context.Context, this *Node) error {
	r := this.getRunnerSnapshot() // snapshot just before execution
	if r == nil {
		return ErrNoRunner
	}
	return r.RunE(ctx, this)
}
