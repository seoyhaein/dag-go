// Package main demonstrates a multi-stage software build pipeline using dag-go.
//
// Graph topology (all nodes succeed):
//
//	start_node
//	    ├─► build_libcore  (~200ms)
//	    ├─► build_libauth  (~180ms)
//	    └─► build_libdb    (~220ms)
//	              ├─► compile_app   (fan-in, waits for all 3 libs, ~300ms)
//	                      ├─► unit_test    (~250ms)
//	                      └─► integ_test   (~350ms)
//	                               └─► package_app  (fan-in, ~150ms)
//	                                       └─► deploy (~100ms)
//
// This example demonstrates:
//
//  1. Multi-level fan-out / fan-in topology
//  2. Bulk runner assignment via SetNodeRunners
//  3. Per-node timeouts via CreateNodeWithTimeOut
//  4. ToMermaid() with runner-type annotations in node labels
//  5. Explicit node pre-creation before edge wiring (AddEdgeIfNodesExist)
//
// Run:
//
//	go run ./examples/build_system/
package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	dag "github.com/seoyhaein/dag-go"
)

// ── Runner implementations ───────────────────────────────────────────────────

// CompileRunner simulates compiling a single library or application module.
type CompileRunner struct {
	target  string
	latency time.Duration
}

func (r *CompileRunner) RunE(ctx context.Context, _ interface{}) error {
	fmt.Printf("  [compile] building %s...\n", r.target)
	select {
	case <-time.After(r.latency):
	case <-ctx.Done():
		return ctx.Err()
	}
	fmt.Printf("  [compile] %s: OK (%.0fms)\n", r.target, float64(r.latency.Milliseconds()))
	return nil
}

// TestRunner simulates running a test suite.
type TestRunner struct {
	suite   string
	latency time.Duration
}

func (r *TestRunner) RunE(ctx context.Context, _ interface{}) error {
	fmt.Printf("  [test] running %s suite...\n", r.suite)
	select {
	case <-time.After(r.latency):
	case <-ctx.Done():
		return ctx.Err()
	}
	fmt.Printf("  [test] %s: all tests passed\n", r.suite)
	return nil
}

// PackageRunner simulates creating a distributable artifact.
type PackageRunner struct{}

func (r *PackageRunner) RunE(ctx context.Context, _ interface{}) error {
	fmt.Println("  [package] creating distributable artifact...")
	select {
	case <-time.After(150 * time.Millisecond):
	case <-ctx.Done():
		return ctx.Err()
	}
	fmt.Println("  [package] artifact created: app-v1.0.0.tar.gz")
	return nil
}

// DeployRunner simulates deploying the packaged artifact to a target environment.
type DeployRunner struct{}

func (r *DeployRunner) RunE(ctx context.Context, _ interface{}) error {
	fmt.Println("  [deploy] deploying to staging environment...")
	select {
	case <-time.After(100 * time.Millisecond):
	case <-ctx.Done():
		return ctx.Err()
	}
	fmt.Println("  [deploy] deployment successful: staging.example.com")
	return nil
}

// ── Main ─────────────────────────────────────────────────────────────────────

func main() {
	fmt.Println("=== Build System Example ===")
	fmt.Println()

	// ── Build the DAG ────────────────────────────────────────────────────────
	d, err := dag.InitDag()
	if err != nil {
		fmt.Printf("InitDag: %v\n", err)
		return
	}

	// Pre-create nodes explicitly so we can attach individual runners before wiring.
	nodeIDs := []string{
		"build_libcore", "build_libauth", "build_libdb",
		"compile_app", "unit_test", "integ_test",
		"package_app", "deploy",
	}
	for _, id := range nodeIDs {
		if d.CreateNode(id) == nil {
			fmt.Printf("CreateNode(%q) failed\n", id)
			return
		}
	}

	// Assign a dedicated Runnable to every node using the bulk-assignment API.
	runners := map[string]dag.Runnable{
		"build_libcore": &CompileRunner{target: "libcore", latency: 200 * time.Millisecond},
		"build_libauth": &CompileRunner{target: "libauth", latency: 180 * time.Millisecond},
		"build_libdb":   &CompileRunner{target: "libdb", latency: 220 * time.Millisecond},
		"compile_app":   &CompileRunner{target: "app (linking)", latency: 300 * time.Millisecond},
		"unit_test":     &TestRunner{suite: "unit", latency: 250 * time.Millisecond},
		"integ_test":    &TestRunner{suite: "integration", latency: 350 * time.Millisecond},
		"package_app":   &PackageRunner{},
		"deploy":        &DeployRunner{},
	}
	applied, missing, skipped := d.SetNodeRunners(runners)
	fmt.Printf("SetNodeRunners: applied=%d  missing=%v  skipped=%v\n\n", applied, missing, skipped)

	// Wire the dependency graph:
	//   3 parallel lib builds → compile_app → {unit_test, integ_test} → package_app → deploy
	edges := []struct{ from, to string }{
		{dag.StartNode, "build_libcore"},
		{dag.StartNode, "build_libauth"},
		{dag.StartNode, "build_libdb"},
		{"build_libcore", "compile_app"},
		{"build_libauth", "compile_app"},
		{"build_libdb", "compile_app"},
		{"compile_app", "unit_test"},
		{"compile_app", "integ_test"},
		{"unit_test", "package_app"},
		{"integ_test", "package_app"},
		{"package_app", "deploy"},
	}
	for _, e := range edges {
		if err := d.AddEdgeIfNodesExist(e.from, e.to); err != nil {
			fmt.Printf("AddEdgeIfNodesExist(%q→%q): %v\n", e.from, e.to, err)
			return
		}
	}

	if err := d.FinishDag(); err != nil {
		if errors.Is(err, dag.ErrCycleDetected) {
			fmt.Printf("build graph has a cycle: %v\n", err)
			return
		}
		fmt.Printf("FinishDag: %v\n", err)
		return
	}

	// ── Print Mermaid diagram ────────────────────────────────────────────────
	fmt.Println("Build graph (Mermaid — paste into https://mermaid.live):")
	fmt.Println("──────────────────────────────────────────────────────────")
	fmt.Println(d.ToMermaid())
	fmt.Println("──────────────────────────────────────────────────────────")
	fmt.Println()

	// ── Execute the build pipeline ───────────────────────────────────────────
	ctx := context.Background()
	d.ConnectRunner()
	if !d.GetReady(ctx) {
		fmt.Println("GetReady failed")
		return
	}

	fmt.Printf("Starting build pipeline (initial progress: %.0f%%)...\n\n", d.Progress()*100)
	start := time.Now()
	d.Start()
	ok := d.Wait(ctx)
	elapsed := time.Since(start)

	fmt.Printf("\nBuild pipeline finished in %v | success=%v | progress=%.0f%%\n",
		elapsed.Round(time.Millisecond), ok, d.Progress()*100)

	if ok {
		fmt.Println("\n  All stages passed — artifact is ready for deployment!")
	} else {
		fmt.Println("\n  Build failed — check logs for the root cause.")
	}
}
