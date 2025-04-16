package dag_go

import (
	"io"
	"testing"
)

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
