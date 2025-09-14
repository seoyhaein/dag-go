package scheduler

import "testing"

// helper: JSON 정규화된 파이프라인에서 in/out degree 계산
func degrees(p *Pipeline) (map[string]int, map[string]int) {
	inDeg := make(map[string]int, len(p.Nodes))
	outDeg := make(map[string]int, len(p.Nodes))
	for id := range p.Nodes {
		inDeg[id] = 0
		outDeg[id] = 0
	}
	for id, n := range p.Nodes {
		for _, dep := range n.Graph.DependsOn {
			inDeg[id]++
			outDeg[dep]++
		}
	}
	return inDeg, outDeg
}

// TestBuildDagFromPipeline0 기본 파이프라인 파일로부터 DAG 생성 테스트
// 일단, dag 생성만 검증하였음. TODO 이후 dag 를 실행하는 부분도 테스트 필요.
func TestBuildDagFromPipeline0(t *testing.T) {
	// 1) parse
	p, err := ParsePipelineFile("../pipeline.jsonc")
	if err != nil {
		t.Fatalf("ParsePipelineFile error: %v", err)
	}
	if p == nil || len(p.Nodes) == 0 {
		t.Fatalf("invalid pipeline: %+v", p)
	}

	// 2) build
	dag, err := BuildDagFromPipeline(p /*, (옵션이 있으면 여기 추가) */)
	if err != nil {
		t.Fatalf("BuildDagFromPipeline error: %v", err)
	}
	if dag == nil {
		t.Fatal("got nil dag")
	}
	// 생성 확인 (이름을 모르더라도 포인터 존재로 체크)
	if dag.StartNode == nil {
		t.Fatal("StartNode is nil")
	}
	if dag.EndNode == nil {
		t.Fatal("EndNode is nil (FinishDag should create it)")
	}

	// 3) init/finalize ID 탐색 (Type 기반)
	var initID, finalizeID string
	var hasInit, hasFinalize bool
	for id, n := range p.Nodes {
		switch n.Type {
		case "init":
			if hasInit {
				t.Fatalf("pipeline contains multiple init nodes: %q and %q", initID, id)
			}
			initID, hasInit = id, true
		case "finalize":
			if hasFinalize {
				t.Fatalf("pipeline contains multiple finalize nodes: %q and %q", finalizeID, id)
			}
			finalizeID, hasFinalize = id, true
		}
	}
	if !hasInit {
		t.Fatal(`init node (type=="init") is required`)
	}

	// 4) JSON 기준 루트/리프 계산 (NormalizeDepends 결과 기반)
	inDeg, outDeg := degrees(p)

	// 5) 배선 검증: "이미 있는 엣지를 다시 추가" 시도 → 실패해야 통과
	//    - StartNode("start_node") -> init
	if err := dag.AddEdgeIfNodesExist("start_node", initID); err == nil {
		t.Fatalf("expected existing edge start_node->%s, but AddEdgeIfNodesExist succeeded", initID)
	}

	//    - init -> 모든 루트(본인 제외)
	for id, deg := range inDeg {
		if id == initID {
			continue
		}
		if deg == 0 {
			if err := dag.AddEdgeIfNodesExist(initID, id); err == nil {
				t.Fatalf("expected existing edge %s->%s (root), but AddEdgeIfNodesExist succeeded", initID, id)
			}
		}
	}

	//    - (있다면) 모든 리프 -> finalize
	if hasFinalize {
		for id, out := range outDeg {
			if id == finalizeID {
				continue
			}
			if out == 0 {
				if err := dag.AddEdgeIfNodesExist(id, finalizeID); err == nil {
					t.Fatalf("expected existing edge %s->%s (leaf), but AddEdgeIfNodesExist succeeded", id, finalizeID)
				}
			}
		}
		//    - finalize -> EndNode("end_node") 연결
		if err := dag.AddEdgeIfNodesExist(finalizeID, "end_node"); err == nil {
			t.Fatalf("expected existing edge %s->end_node, but AddEdgeIfNodesExist succeeded", finalizeID)
		}
	}

	// 추가 안전망(선택): 사이클 없어야 함
	// 주의: DetectCycle는 dag_go에 있으니, import 없이 쓰려면 생략 가능.
	// if dag_go.DetectCycle(dag) {
	// 	t.Fatal("cycle detected in DAG")
	// }
}
