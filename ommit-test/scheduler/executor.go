package scheduler

import (
	"errors"
	"fmt"
	dag_go "github.com/seoyhaein/dag-go"
)

// pipeline 이 들어오면 dag 로 바꿔줌.

// BuildDagFromPipeline ParsePipeline로 얻은 *Pipeline을 DAG로 변환
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
