package dag_go

import "context"

// Runnable defines the interface for executable units attached to a DAG node.
// RunE is invoked during the inFlight phase.  The supplied ctx carries the
// cancellation / timeout signal from the enclosing DAG execution; implementations
// should honour ctx.Done() so that long-running work can be interrupted cleanly.
// The second parameter is the *Node that is currently executing, passed as an
// opaque interface{} to avoid an import cycle for callers outside this package.
type Runnable interface {
	RunE(ctx context.Context, a interface{}) error
}
