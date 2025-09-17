package scheduler

import (
	"errors"
	"fmt"
	"sort"

	dag_go "github.com/seoyhaein/dag-go"
)

// 추후 package
// pipeline 이 들어오면 dag 로 바꿔줌.
// scheduler 와 grpc 통신 해줘야 함.
// task 를 가져올때, executor 에서 pull 하는 방식으로 해야 안정적임.

// BuildDagFromPipeline ParsePipeline로 얻은 *Pipeline을 DAG로 변환
func BuildDagFromPipeline(p *Pipeline, dagOpts ...dag_go.DagOption) (*dag_go.Dag, error) {
	if p == nil || len(p.Nodes) == 0 {
		return nil, errors.New("pipeline is nil or empty")
	}

	// DAG 생성 (StartNode 포함)
	dag, err := dag_go.InitDagWithOptions(dagOpts...)
	if err != nil {
		return nil, fmt.Errorf("init dag: %w", err)
	}

	// 노드 ID 결정론 순회 준비
	ids := make([]string, 0, len(p.Nodes))
	for id := range p.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	// 파이프라인 노드 선생성 (+ 예약어 차단, StartNode, EndNode)
	for _, id := range ids {
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

	// 의존성 엣지 생성 + in/out degree 집계 (JSON 정규화 기준) inDeg 는 부모, outDeg 자식 부모가 0 이면 루트, 자식이 0이면 리프
	inDeg := make(map[string]int, len(p.Nodes))
	outDeg := make(map[string]int, len(p.Nodes))
	for _, id := range ids {
		inDeg[id] = 0
		outDeg[id] = 0
	}
	for _, id := range ids {
		n := p.Nodes[id]
		// n.Graph.DependsOn 은 NormalizeDepends 에서 이미 정렬/디듀프/검증됨
		for _, dep := range n.Graph.DependsOn {
			if err := dag.AddEdgeIfNodesExist(dep, id); err != nil {
				return nil, fmt.Errorf("add edge %s->%s: %w", dep, id, err)
			}
			inDeg[id]++
			outDeg[dep]++
		}
	}

	// init/finalize 탐색(타입 기반, 고유성 보장) 중복일때 오류 검증.
	var (
		initID, finalizeID   string
		hasInit, hasFinalize bool
	)
	for _, id := range ids {
		switch p.Nodes[id].Type {
		case "init":
			if hasInit {
				return nil, fmt.Errorf("multiple init nodes: %q and %q", initID, id)
			}
			initID, hasInit = id, true
		case "finalize":
			if hasFinalize {
				return nil, fmt.Errorf("multiple finalize nodes: %q and %q", finalizeID, id)
			}
			finalizeID, hasFinalize = id, true
		}
	}

	// 제약: init은 루트(inDeg==0), finalize는 리프(outDeg==0)
	if !hasInit {
		return nil, fmt.Errorf("init node (type==\"init\") is required")
	}
	if inDeg[initID] != 0 {
		return nil, fmt.Errorf("init node %q must not have dependencies (inDeg=%d)", initID, inDeg[initID])
	}
	if hasFinalize && outDeg[finalizeID] != 0 {
		return nil, fmt.Errorf("finalize node %q must not be a dependency of any node (outDeg=%d)", finalizeID, outDeg[finalizeID])
	}

	// 루트 연결: StartNode->init, init->(모든 루트)
	if err := dag.AddEdgeIfNodesExist(dag_go.StartNode, initID); err != nil {
		return nil, fmt.Errorf("connect start_node->%s: %w", initID, err)
	}
	for _, id := range ids {
		if id == initID {
			continue
		}
		if inDeg[id] == 0 {
			if err := dag.AddEdgeIfNodesExist(initID, id); err != nil {
				return nil, fmt.Errorf("connect %s->%s: %w", initID, id, err)
			}
		}
	}

	// 리프 → finalize (있을 때만) — JSON 기준 outDeg==0
	if hasFinalize {
		for _, id := range ids {
			if id == finalizeID {
				continue
			}
			if outDeg[id] == 0 {
				if err := dag.AddEdgeIfNodesExist(id, finalizeID); err != nil {
					return nil, fmt.Errorf("connect %s->%s: %w", id, finalizeID, err)
				}
			}
		}
		// finalize는 리프가 되므로 FinishDag가 finalize->EndNode 연결
	}

	// FinishDag: leaf→EndNode 연결 + 사이클 검사
	if err := dag.FinishDag(); err != nil {
		return nil, fmt.Errorf("finish dag: %w", err)
	}
	return dag, nil
}
