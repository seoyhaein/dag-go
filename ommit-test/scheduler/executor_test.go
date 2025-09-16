package scheduler

import (
	"context"
	"errors"
	"fmt"
	dag_go "github.com/seoyhaein/dag-go"
	"testing"
	"time"
)

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

// 10초 후 성공을 리턴하는 목(mock) 러너
type mockRunner struct{}

func (mockRunner) RunE(n *dag_go.Node) error {
	time.Sleep(10 * time.Second)
	return nil
}

func TestPipeline_RunWithMockSpawner(t *testing.T) {
	p, _ := ParsePipelineFile("../pipeline.jsonc")
	_, _ = BuildDagFromPipeline(p /*, (옵션이 있으면 여기 추가) */)

}

// TODO RunE 를 node 의 정보를 가져와서,
// TODO 컨테이너를 만들어주는 api 로 재작성 하면 됨.
// TODO 기타, 여러 부가적인 기능들을 만들어주면 될듯하다.

// 전역/노드 주입용 러너들: 실행 시 해당 노드 ID를 출력
// Runnable 구현: a를 *Node로 캐스팅해 ID 출력
type echoOK struct{ d time.Duration }

func (r echoOK) RunE(a interface{}) error {
	id := "<unknown>"
	if n, ok := a.(*dag_go.Node); ok && n != nil {
		id = n.ID
	}
	fmt.Printf("[RunE OK] node=%s sleep=%s\n", id, r.d)
	time.Sleep(r.d)
	return nil
}

type echoFail struct{ d time.Duration }

func (r echoFail) RunE(a interface{}) error {
	id := "<unknown>"
	if n, ok := a.(*dag_go.Node); ok && n != nil {
		id = n.ID
	}
	fmt.Printf("[RunE FAIL] node=%s sleep=%s\n", id, r.d)
	time.Sleep(r.d)
	return errors.New("mock failure")
}

// 전역 주입 + 특정 노드 오버라이드(모두 성공)
func TestDAG_GlobalAndPerNodeInjection_PrintIDs_Success(t *testing.T) {
	dag, err := dag_go.InitDag()
	if err != nil {
		t.Fatalf("InitDag error: %v", err)
	}

	// DAG 전역 러너 주입: 기본은 100ms 후 성공, 각 노드 실행 시 ID 출력됨
	dag.SetContainerCmd(echoOK{d: 100 * time.Millisecond})

	// 노드 생성
	a := dag.CreateNode("A")
	b1 := dag.CreateNode("B1")
	b2 := dag.CreateNode("B2")
	c := dag.CreateNode("C")

	// 엣지 연결: start -> A -> (B1,B2) -> C -> end(자동 연결됨)
	if err := dag.AddEdge(dag_go.StartNode, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := dag.AddEdge(a.ID, b1.ID); err != nil {
		t.Fatal(err)
	}
	if err := dag.AddEdge(a.ID, b2.ID); err != nil {
		t.Fatal(err)
	}
	if err := dag.AddEdge(b1.ID, c.ID); err != nil {
		t.Fatal(err)
	}
	if err := dag.AddEdge(b2.ID, c.ID); err != nil {
		t.Fatal(err)
	}

	// 노드별 주입: B2만 더 긴 딜레이로 오버라이드(실행 시 node=B2 출력)
	b2.RunCommand = echoOK{d: 300 * time.Millisecond}

	// 실행
	if err := dag.FinishDag(); err != nil {
		t.Fatalf("FinishDag: %v", err)
	}
	if !dag.ConnectRunner() {
		t.Fatal("ConnectRunner failed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if !dag.GetReady(ctx) {
		t.Fatal("GetReady failed")
	}
	if !dag.Start() {
		t.Fatal("Start failed")
	}
	if !dag.Wait(ctx) {
		t.Fatal("Wait failed (expected success)")
	}
}
