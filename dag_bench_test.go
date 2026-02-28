package dag_go

import (
	"fmt"
	"io"
	"testing"
)

// ── copyDag benchmarks ────────────────────────────────────────────────────────
// These benchmarks verify that the pre-allocation of parent/children slices
// in copyDag reduces the number of heap allocations per call.

func BenchmarkCopyDag_Small(b *testing.B) {
	Log.SetOutput(io.Discard)
	// 소형 DAG 생성: 10개의 노드, 간선 추가 확률 0.5
	dag := generateDAG(10, 0.5)
	// Warm-up: 벤치마크 루프 전에 한 번 실행
	_, _ = copyDag(dag)
	// 메모리 할당 통계 리포트 활성화
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		newNodes, newEdges := copyDag(dag)
		if len(newNodes) == 0 || (len(dag.Edges) > 0 && len(newEdges) == 0) {
			b.Fatal("copyDag failed")
		}
	}
}

func BenchmarkCopyDag_Medium(b *testing.B) {
	Log.SetOutput(io.Discard)
	dag := generateDAG(100, 0.3)
	// Warm-up: 벤치마크 루프 전에 한 번 실행
	_, _ = copyDag(dag)
	// 메모리 할당 통계 리포트 활성화
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		newNodes, newEdges := copyDag(dag)
		if len(newNodes) == 0 || (len(dag.Edges) > 0 && len(newEdges) == 0) {
			b.Fatal("copyDag failed")
		}
	}
}

func BenchmarkCopyDag_Large(b *testing.B) {
	Log.SetOutput(io.Discard)
	dag := generateDAG(1000, 0.1)
	// Warm-up: 벤치마크 루프 전에 한 번 실행
	_, _ = copyDag(dag)
	// 메모리 할당 통계 리포트 활성화
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		newNodes, newEdges := copyDag(dag)
		if len(newNodes) == 0 || (len(dag.Edges) > 0 && len(newEdges) == 0) {
			b.Fatal("copyDag failed")
		}
	}
}

// ── detectCycle benchmarks ────────────────────────────────────────────────────
// These benchmarks verify that pre-sizing visited/recStack maps reduces
// allocation overhead during DFS traversal.

func BenchmarkDetectCycle_Small(b *testing.B) {
	Log.SetOutput(io.Discard)
	// 10 nodes, edge probability 0.3 — acyclic by construction (i < j only)
	dag := generateDAG(10, 0.3)
	// Warm-up
	_ = detectCycle(dag)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if detectCycle(dag) {
			b.Fatal("unexpected cycle in generated DAG")
		}
	}
}

func BenchmarkDetectCycle_Medium(b *testing.B) {
	Log.SetOutput(io.Discard)
	dag := generateDAG(100, 0.2)
	_ = detectCycle(dag)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if detectCycle(dag) {
			b.Fatal("unexpected cycle in generated DAG")
		}
	}
}

func BenchmarkDetectCycle_Large(b *testing.B) {
	Log.SetOutput(io.Discard)
	dag := generateDAG(1000, 0.05)
	_ = detectCycle(dag)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if detectCycle(dag) {
			b.Fatal("unexpected cycle in generated DAG")
		}
	}
}

// ── ToMermaid benchmarks ──────────────────────────────────────────────────────
// These benchmarks measure the cost of generating a Mermaid diagram string,
// verifying that the pre-sized defined-map and the extracted helper functions
// keep allocations proportional to graph size.

// buildMermaidDag creates a fan-out DAG suitable for ToMermaid benchmarking:
//
//	start_node → node0, node1, … node(width-1)
//
// FinishDag automatically connects all leaf nodes to the synthetic end_node,
// so callers must NOT manually wire to EndNode.
func buildMermaidDag(b *testing.B, width int) *Dag {
	b.Helper()
	d, err := InitDag()
	if err != nil {
		b.Fatalf("InitDag: %v", err)
	}
	for i := range width {
		id := fmt.Sprintf("node%d", i)
		if err := d.AddEdge(StartNode, id); err != nil {
			b.Fatalf("AddEdge start→%s: %v", id, err)
		}
	}
	if err := d.FinishDag(); err != nil {
		b.Fatalf("FinishDag: %v", err)
	}
	return d
}

func BenchmarkToMermaid_Small(b *testing.B) {
	Log.SetOutput(io.Discard)
	// 5 user nodes: start → {A,B,C,D,E} → end  (12 edges total after FinishDag)
	d := buildMermaidDag(b, 5)
	_ = d.ToMermaid() // Warm-up
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = d.ToMermaid()
	}
}

func BenchmarkToMermaid_Medium(b *testing.B) {
	Log.SetOutput(io.Discard)
	// 20 user nodes
	d := buildMermaidDag(b, 20)
	_ = d.ToMermaid()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = d.ToMermaid()
	}
}

func BenchmarkToMermaid_Large(b *testing.B) {
	Log.SetOutput(io.Discard)
	// 26 user nodes (full alphabet — keeps IDs single-character)
	d := buildMermaidDag(b, 26)
	_ = d.ToMermaid()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = d.ToMermaid()
	}
}
