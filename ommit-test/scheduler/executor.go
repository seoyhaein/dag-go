package scheduler

import (
	"errors"
	"fmt"
	dag_go "github.com/seoyhaein/dag-go"
)

// pipeline 이 들어오면 dag 로 바꿔줌.

// BuildDagFromPipeline ParsePipeline로 얻은 *Pipeline을 DAG로 변환
// TODO 일단 dag-go 에서는 StartNode, EndNode 를 직접 넣어 주었는데, 여기서 json 에서 start 노드와 end 노드가 있어서 중복되어 버림.
func BuildDagFromPipeline(p *Pipeline, dagOpts ...dag_go.DagOption) (*dag_go.Dag, error) {
	if p == nil {
		return nil, errors.New("pipeline is nil")
	}
	if len(p.Nodes) == 0 {
		return nil, errors.New("pipeline has no nodes")
	}

	// 1) DAG 생성 (StartNode 포함)
	dag, err := dag_go.InitDagWithOptions(dagOpts...)
	if err != nil {
		return nil, fmt.Errorf("init dag: %w", err)
	}

	// 2) 파이프라인 노드 선생성
	for id := range p.Nodes {
		if id == "" {
			return nil, fmt.Errorf("empty node id in pipeline")
		}
		if dag.CreateNode(id) == nil {
			// 이미 존재 → 중복 id 가능성 or Start/End 예약어 충돌
			// (StartNode/EndNode는 내부 예약어이므로 피하는 게 안전)
			return nil, fmt.Errorf("failed to create node: %s (duplicate or reserved?)", id)
		}
	}

	// 3) 의존성 검증 + 엣지 생성(dep -> node)
	inDeg := make(map[string]int, len(p.Nodes))
	for id := range p.Nodes {
		inDeg[id] = 0
	}
	for id, n := range p.Nodes {
		for _, dep := range n.Graph.DependsOn {
			if _, ok := p.Nodes[dep]; !ok {
				return nil, fmt.Errorf("node %q depends on unknown node %q", id, dep)
			}
			if err := dag.AddEdge(dep, id); err != nil {
				return nil, fmt.Errorf("add edge %s->%s: %w", dep, id, err)
			}
			inDeg[id]++
		}
	}

	// 4) 루트(의존 0)들을 StartNode에 연결
	for id, deg := range inDeg {
		if deg == 0 {
			// 파이프라인 JSON에 type:"start"가 있든 없든,
			// 실행의 진입점은 내부 StartNode로 통일하는 편이 깔끔.
			if err := dag.AddEdge(dag_go.StartNode, id); err != nil {
				return nil, fmt.Errorf("connect start->%s: %w", id, err)
			}
		}
	}

	// 5) FinishDag: leaf→EndNode 연결 + 사이클 검사
	if err := dag.FinishDag(); err != nil {
		return nil, fmt.Errorf("finish dag: %w", err)
	}

	return dag, nil
}

// BuildDagFromPipelineA TODO 테스트 진행
// BuildDagFromPipelineA TODO 테스트 진행.
func BuildDagFromPipelineA(p *Pipeline, dagOpts ...dag_go.DagOption) (*dag_go.Dag, error) {
	if p == nil {
		return nil, errors.New("pipeline is nil")
	}
	if len(p.Nodes) == 0 {
		return nil, errors.New("pipeline has no nodes")
	}

	// 1) DAG 생성 (StartNode 포함)
	dag, err := dag_go.InitDagWithOptions(dagOpts...)
	if err != nil {
		return nil, fmt.Errorf("init dag: %w", err)
	}

	// 2) 파이프라인 노드 선생성 (+ 예약어 차단)
	for id := range p.Nodes {
		if id == "" {
			return nil, fmt.Errorf("empty node id in pipeline")
		}
		if id == dag_go.StartNode || id == dag_go.EndNode {
			return nil, fmt.Errorf("node id %q is reserved", id)
		}
		if dag.CreateNode(id) == nil {
			return nil, fmt.Errorf("duplicate node id or reserved: %s", id)
		}
	}

	// 3) 의존성 검증 + 엣지 생성(dep -> node), 디듀프 + in/out degree 집계
	inDeg := make(map[string]int, len(p.Nodes))
	outDeg := make(map[string]int, len(p.Nodes))
	for id := range p.Nodes {
		inDeg[id] = 0
		outDeg[id] = 0
	}

	for id, n := range p.Nodes {
		seen := make(map[string]struct{}, len(n.Graph.DependsOn))
		for _, dep := range n.Graph.DependsOn {
			if dep == id {
				return nil, fmt.Errorf("node %q depends on itself", id)
			}
			if _, ok := p.Nodes[dep]; !ok {
				return nil, fmt.Errorf("node %q depends on unknown node %q", id, dep)
			}
			if _, dup := seen[dep]; dup {
				continue // 중복 의존성 무시
			}
			seen[dep] = struct{}{}

			if err := dag.AddEdgeIfNodesExist(dep, id); err != nil {
				return nil, fmt.Errorf("add edge %s->%s: %w", dep, id, err)
			}
			inDeg[id]++
			outDeg[dep]++
		}
	}

	// 4) init/finalize 후보 결정 (이름 규약: "start"=init, "end"=finalize)
	initID, hasInit := "", false
	if _, ok := p.Nodes["start"]; ok {
		initID, hasInit = "start", true
	}
	finalizeID, hasFinalize := "", false
	if _, ok := p.Nodes["end"]; ok {
		finalizeID, hasFinalize = "end", true
	}

	// 5) 루트(의존 0) 연결: init 있으면 StartNode->init, init->roots; 없으면 StartNode->roots
	if hasInit {
		if err := dag.AddEdgeIfNodesExist(dag_go.StartNode, initID); err != nil {
			return nil, fmt.Errorf("connect start_node->%s: %w", initID, err)
		}
		for id, deg := range inDeg {
			if deg == 0 && id != initID {
				if err := dag.AddEdgeIfNodesExist(initID, id); err != nil {
					return nil, fmt.Errorf("connect %s->%s: %w", initID, id, err)
				}
				// 참고: in/outDeg 업데이트는 선택사항(아래 리프 계산은 JSON 기준 outDeg만 사용)
			}
		}
	} else {
		for id, deg := range inDeg {
			if deg == 0 {
				if err := dag.AddEdgeIfNodesExist(dag_go.StartNode, id); err != nil {
					return nil, fmt.Errorf("connect start_node->%s: %w", id, err)
				}
			}
		}
	}

	// 6) 리프(자식 0) 연결: finalize 있으면 (모든 리프)->finalize
	//    리프 판단은 JSON 의존성 기준 outDeg==0으로 결정 (중복 엣지 방지에 유리)
	if hasFinalize {
		for id, out := range outDeg {
			if out == 0 && id != finalizeID {
				if err := dag.AddEdgeIfNodesExist(id, finalizeID); err != nil {
					return nil, fmt.Errorf("connect %s->%s: %w", id, finalizeID, err)
				}
			}
		}
		// FinishDag 가 finalize -> EndNode 를 자동으로 연결해준다 (finalize가 리프가 되므로)
	}

	// 7) FinishDag: leaf→EndNode 연결 + 사이클 검사
	if err := dag.FinishDag(); err != nil {
		return nil, fmt.Errorf("finish dag: %w", err)
	}

	return dag, nil
}
