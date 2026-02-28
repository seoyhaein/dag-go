// Package main demonstrates dag-go's Reset() method for implementing a
// retry strategy without rebuilding the graph.
//
// Graph topology (simple 3-node pipeline):
//
//	start_node → fetch → process → save → end_node
//
// Strategy:
//
//  1. First execution — "fetch" uses a TransientRunner that fails on the first
//     call, simulating a transient network error.  Because "fetch" fails,
//     "process" and "save" are automatically Skipped and Wait returns false.
//
//  2. Reset() — restores all node statuses to Pending, recreates edge channels,
//     and rebuilds vertex wiring.  The graph topology and runner assignments are
//     preserved; no need to call InitDag / AddEdge / FinishDag again.
//
//  3. Second execution — TransientRunner succeeds on the second call (its
//     internal counter is now > 1).  All nodes complete successfully and Wait
//     returns true.
//
// This example demonstrates:
//
//   - dag.Reset() for zero-cost DAG reuse
//   - Atomic attempt tracking inside a Runnable
//   - Progress() reflecting the state before/after each run
//   - The full lifecycle: ConnectRunner → GetReady → Start → Wait
//
// Run:
//
//	go run ./examples/reset_retry/
package main

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	dag "github.com/seoyhaein/dag-go"
)

// ── Runner implementations ───────────────────────────────────────────────────

// TransientRunner simulates a network call that fails on the very first
// invocation and succeeds on every subsequent call.  An atomic int32 counter
// makes the implementation safe when goroutines call RunE concurrently.
type TransientRunner struct {
	name  string
	count int32 // incremented atomically on each RunE invocation
}

func (r *TransientRunner) RunE(ctx context.Context, _ interface{}) error {
	n := atomic.AddInt32(&r.count, 1)
	fmt.Printf("  [%s] attempt #%d\n", r.name, n)
	if n == 1 {
		return fmt.Errorf("[%s] attempt #%d failed: transient connection error", r.name, n)
	}
	select {
	case <-time.After(80 * time.Millisecond):
	case <-ctx.Done():
		return ctx.Err()
	}
	fmt.Printf("  [%s] attempt #%d succeeded\n", r.name, n)
	return nil
}

// StableRunner always succeeds — used for nodes that are not the retry target.
type StableRunner struct{ name string }

func (r *StableRunner) RunE(ctx context.Context, _ interface{}) error {
	fmt.Printf("  [%s] running...\n", r.name)
	select {
	case <-time.After(60 * time.Millisecond):
	case <-ctx.Done():
		return ctx.Err()
	}
	fmt.Printf("  [%s] done\n", r.name)
	return nil
}

// ── Run helper ───────────────────────────────────────────────────────────────

// executeRun wires runners, launches workers, starts the DAG, and waits for
// completion.  It returns true when every node succeeded.
func executeRun(d *dag.Dag, label string) bool {
	fmt.Printf("\n── %s ──\n", label)
	fmt.Printf("  Progress before start: %.0f%%\n", d.Progress()*100)

	ctx := context.Background()
	d.ConnectRunner()
	if !d.GetReady(ctx) {
		fmt.Println("  GetReady failed")
		return false
	}

	start := time.Now()
	d.Start()
	ok := d.Wait(ctx)
	elapsed := time.Since(start)

	fmt.Printf("  Progress after Wait:   %.0f%%\n", d.Progress()*100)
	fmt.Printf("  Result: success=%v  elapsed=%v\n", ok, elapsed.Round(time.Millisecond))
	return ok
}

// ── Main ─────────────────────────────────────────────────────────────────────

func main() {
	fmt.Println("=== Reset & Retry Example ===")
	fmt.Println()

	// ── Build the pipeline (once; reused across retries) ─────────────────────
	d, err := dag.InitDag()
	if err != nil {
		fmt.Printf("InitDag: %v\n", err)
		return
	}

	// Create nodes explicitly to enable per-node runner assignment.
	for _, id := range []string{"fetch", "process", "save"} {
		if d.CreateNode(id) == nil {
			fmt.Printf("CreateNode(%q) failed\n", id)
			return
		}
	}

	// "fetch" gets a transient runner; the other nodes always succeed.
	fetchRunner := &TransientRunner{name: "fetch"}
	runners := map[string]dag.Runnable{
		"fetch":   fetchRunner,
		"process": &StableRunner{name: "process"},
		"save":    &StableRunner{name: "save"},
	}
	if _, missing, _ := d.SetNodeRunners(runners); len(missing) > 0 {
		fmt.Printf("SetNodeRunners: missing nodes %v\n", missing)
		return
	}

	// Wire: start → fetch → process → save
	for _, e := range []struct{ from, to string }{
		{dag.StartNode, "fetch"},
		{"fetch", "process"},
		{"process", "save"},
	} {
		if err := d.AddEdgeIfNodesExist(e.from, e.to); err != nil {
			fmt.Printf("AddEdgeIfNodesExist(%q→%q): %v\n", e.from, e.to, err)
			return
		}
	}

	if err := d.FinishDag(); err != nil {
		if errors.Is(err, dag.ErrCycleDetected) {
			fmt.Printf("pipeline has a cycle: %v\n", err)
			return
		}
		fmt.Printf("FinishDag: %v\n", err)
		return
	}

	fmt.Println("Pipeline graph (Mermaid):")
	fmt.Println(d.ToMermaid())

	// ── Attempt 1 (expected: FAIL) ────────────────────────────────────────────
	ok1 := executeRun(d, "Attempt 1 — transient failure expected")
	if ok1 {
		fmt.Println("  (unexpected success on attempt 1)")
		return
	}
	fmt.Println("  ✗ Attempt 1 failed as expected — scheduling retry via Reset()")

	// ── Reset: restores graph execution state without rebuilding topology ─────
	fmt.Println()
	fmt.Println("Calling dag.Reset()...")
	d.Reset()
	fmt.Printf("Progress after Reset(): %.0f%%\n", d.Progress()*100)

	// ── Attempt 2 (expected: SUCCESS) ────────────────────────────────────────
	ok2 := executeRun(d, "Attempt 2 — should succeed")
	if ok2 {
		fmt.Println()
		fmt.Println("  ✓ Retry succeeded — all nodes completed successfully")
		fmt.Printf("  fetch runner total invocations: %d\n", atomic.LoadInt32(&fetchRunner.count))
	} else {
		fmt.Println("  ✗ Retry also failed — manual intervention required")
	}
}
