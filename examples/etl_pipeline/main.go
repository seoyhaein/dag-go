// Package main demonstrates a realistic ETL (Extract-Transform-Load) pipeline
// built with dag-go.
//
// Graph topology:
//
//	start_node
//	    ├─► ingest_db   (DB extraction, ~150ms)
//	    ├─► ingest_api  (REST API fetch, ~250ms)  ← FAILS on purpose
//	    └─► ingest_csv  (CSV file read, ~100ms)
//	             └─► validate (fan-in, waits for all 3)
//	                     └─► transform
//	                             └─► load
//	                                     └─► report
//
// Because ingest_api returns an error, validate (and everything downstream)
// is automatically Skipped by dag-go's status propagation.  The example shows:
//
//  1. Fan-out / fan-in topology (3 parallel extractions)
//  2. Per-node runner overrides via SetNodeRunner
//  3. Automatic error propagation (node failure cascades to children)
//  4. dag.Progress() before / after execution
//  5. dag.ToMermaid() graph visualization
//
// Run:
//
//	go run ./examples/etl_pipeline/
package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	dag "github.com/seoyhaein/dag-go"
)

// ── Runner implementations ───────────────────────────────────────────────────

// IngestRunner simulates extracting data from a named source.
// When shouldFail is true it returns an error to demonstrate failure handling.
type IngestRunner struct {
	source     string
	latency    time.Duration
	shouldFail bool
}

func (r *IngestRunner) RunE(ctx context.Context, _ interface{}) error {
	fmt.Printf("  [%s] starting extraction...\n", r.source)
	select {
	case <-time.After(r.latency):
	case <-ctx.Done():
		return ctx.Err()
	}
	if r.shouldFail {
		return fmt.Errorf("[%s] connection refused: API service unavailable", r.source)
	}
	fmt.Printf("  [%s] extraction complete\n", r.source)
	return nil
}

// ValidateRunner simulates data-quality validation across all ingested records.
type ValidateRunner struct{}

func (r *ValidateRunner) RunE(ctx context.Context, _ interface{}) error {
	fmt.Println("  [validate] validating all ingested records...")
	select {
	case <-time.After(120 * time.Millisecond):
	case <-ctx.Done():
		return ctx.Err()
	}
	fmt.Println("  [validate] validation passed: 0 quality issues detected")
	return nil
}

// TransformRunner applies business-rule transformations.
type TransformRunner struct{}

func (r *TransformRunner) RunE(ctx context.Context, _ interface{}) error {
	fmt.Println("  [transform] applying business rules and normalisation...")
	select {
	case <-time.After(200 * time.Millisecond):
	case <-ctx.Done():
		return ctx.Err()
	}
	fmt.Println("  [transform] transformation complete")
	return nil
}

// LoadRunner writes transformed data to the destination warehouse.
type LoadRunner struct{}

func (r *LoadRunner) RunE(ctx context.Context, _ interface{}) error {
	fmt.Println("  [load] writing to data warehouse...")
	select {
	case <-time.After(150 * time.Millisecond):
	case <-ctx.Done():
		return ctx.Err()
	}
	fmt.Println("  [load] load complete: all records committed")
	return nil
}

// ReportRunner generates a summary report of the pipeline run.
type ReportRunner struct{}

func (r *ReportRunner) RunE(ctx context.Context, _ interface{}) error {
	fmt.Println("  [report] generating pipeline summary report...")
	select {
	case <-time.After(50 * time.Millisecond):
	case <-ctx.Done():
		return ctx.Err()
	}
	fmt.Println("  [report] report written to /tmp/etl_report.json")
	return nil
}

// ── Pipeline builder ─────────────────────────────────────────────────────────

func buildPipeline(apiShouldFail bool) (*dag.Dag, error) {
	d, err := dag.InitDag()
	if err != nil {
		return nil, fmt.Errorf("InitDag: %w", err)
	}

	// Pre-create nodes so per-node runners can be attached before wiring edges.
	nodes := []string{"ingest_db", "ingest_api", "ingest_csv", "validate", "transform", "load", "report"}
	for _, id := range nodes {
		if d.CreateNode(id) == nil {
			return nil, fmt.Errorf("CreateNode(%q) returned nil", id)
		}
	}

	// Attach a specific Runnable to each node.
	runners := map[string]dag.Runnable{
		"ingest_db":  &IngestRunner{source: "database", latency: 150 * time.Millisecond},
		"ingest_api": &IngestRunner{source: "REST API", latency: 250 * time.Millisecond, shouldFail: apiShouldFail},
		"ingest_csv": &IngestRunner{source: "CSV files", latency: 100 * time.Millisecond},
		"validate":   &ValidateRunner{},
		"transform":  &TransformRunner{},
		"load":       &LoadRunner{},
		"report":     &ReportRunner{},
	}
	applied, missing, _ := d.SetNodeRunners(runners)
	if len(missing) > 0 {
		return nil, fmt.Errorf("SetNodeRunners: nodes not found: %v", missing)
	}
	fmt.Printf("  Attached runners to %d nodes\n", applied)

	// Wire the graph: fan-out from start to 3 parallel extractions, then
	// fan-in through validate into the sequential transform → load → report chain.
	edges := []struct{ from, to string }{
		{dag.StartNode, "ingest_db"},
		{dag.StartNode, "ingest_api"},
		{dag.StartNode, "ingest_csv"},
		{"ingest_db", "validate"},
		{"ingest_api", "validate"},
		{"ingest_csv", "validate"},
		{"validate", "transform"},
		{"transform", "load"},
		{"load", "report"},
	}
	for _, e := range edges {
		if err := d.AddEdgeIfNodesExist(e.from, e.to); err != nil {
			return nil, fmt.Errorf("AddEdgeIfNodesExist(%q→%q): %w", e.from, e.to, err)
		}
	}

	if err := d.FinishDag(); err != nil {
		if errors.Is(err, dag.ErrCycleDetected) {
			return nil, fmt.Errorf("pipeline has a cycle: %w", err)
		}
		return nil, fmt.Errorf("FinishDag: %w", err)
	}
	return d, nil
}

// ── Main ─────────────────────────────────────────────────────────────────────

func main() {
	fmt.Println("=== ETL Pipeline Example ===")
	fmt.Println()

	// ── Scenario 1: successful run (all sources available) ───────────────────
	fmt.Println("── Scenario 1: all sources available ──")
	d, err := buildPipeline(false /* apiShouldFail */)
	if err != nil {
		fmt.Printf("Pipeline build failed: %v\n", err)
		return
	}

	fmt.Println("\nMermaid topology (copy into https://mermaid.live):")
	fmt.Println("────────────────────────────────────────────────")
	fmt.Println(d.ToMermaid())
	fmt.Println("────────────────────────────────────────────────")

	fmt.Printf("Progress before execution: %.0f%%\n\n", d.Progress()*100)

	ctx := context.Background()
	d.ConnectRunner()
	d.GetReady(ctx)
	start := time.Now()
	d.Start()
	ok := d.Wait(ctx)

	elapsed := time.Since(start)
	fmt.Printf("\nPipeline finished in %v | success=%v | progress=%.0f%%\n",
		elapsed.Round(time.Millisecond), ok, d.Progress()*100)

	// ── Scenario 2: one source fails → downstream automatically skipped ──────
	fmt.Println()
	fmt.Println("── Scenario 2: REST API source unavailable ──")
	d2, err := buildPipeline(true /* apiShouldFail */)
	if err != nil {
		fmt.Printf("Pipeline build failed: %v\n", err)
		return
	}

	ctx2 := context.Background()
	d2.ConnectRunner()
	d2.GetReady(ctx2)
	fmt.Println()
	start2 := time.Now()
	d2.Start()
	ok2 := d2.Wait(ctx2)

	elapsed2 := time.Since(start2)
	fmt.Printf("\nPipeline finished in %v | success=%v | progress=%.0f%%\n",
		elapsed2.Round(time.Millisecond), ok2, d2.Progress()*100)
	fmt.Println("  → ingest_api failed; validate, transform, load, report were automatically skipped")
}
