package dag_go

import (
	"context"
	"testing"
)

// internal methods 테스트 기준. 특정 exported method 에서 한번 사용되면 해당 메서드를 테스트 해서 간접 테스트함.
// 하지만, 여러번 쓰이면 독립적으로 테스트 함.

func TestInitDag(t *testing.T) {
	// InitDag 호출하여 새로운 Dag 인스턴스 생성
	dag, err := InitDag()
	if err != nil {
		t.Fatalf("InitDag failed: %v", err)
	}
	if dag == nil {
		t.Fatal("InitDag returned nil")
	}

	// StartNode 가 nil 이 아닌지 확인
	if dag.StartNode == nil {
		t.Fatal("StartNode is nil")
	}

	// StartNode 의 Id가 올바르게 설정되었는지 확인 (예: StartNode 상수와 일치하는지)
	if dag.StartNode.Id != StartNode {
		t.Errorf("Expected StartNode Id to be %s, got %s", StartNode, dag.StartNode.Id)
	}

	// StartNode 에 parentVertex 채널이 추가되었는지 확인
	if len(dag.StartNode.parentVertex) == 0 {
		t.Error("StartNode's parentVertex channel is not set")
	} else {
		// 채널이 올바른 용량으로 생성되었는지 확인
		ch := dag.StartNode.parentVertex[0]
		if cap(ch) != Min {
			t.Errorf("Expected parentVertex channel capacity to be %d, got %d", Min, cap(ch))
		}
	}

	// RunningStatus 채널의 용량도 검사
	if cap(dag.RunningStatus) != Max {
		t.Errorf("Expected RunningStatus channel capacity to be %d, got %d", Max, cap(dag.RunningStatus))
	}
}

// DummyRunnable 은 Runnable 인터페이스의 더미 구현체
type DummyRunnable struct{}

// DummyRunnable 이 Runnable 인터페이스를 구현하도록 필요한 메서드들을 정의
func (d DummyRunnable) Run() error {
	return nil
}

func (d DummyRunnable) RunE(a interface{}) (int, error) {
	return 0, nil
}

func (d DummyRunnable) CreateImage(a interface{}, healthChecker string) error {
	return nil
}

func TestCreateNode(t *testing.T) {
	// -------------------------------
	// Case 1: ContainerCmd 가 nil 인 경우
	// -------------------------------
	// NewDag()를 호출하면 기본적으로 ContainerCmd 는 nil
	dag := NewDag()
	id := "test1"
	node := dag.createNode(id)
	if node == nil {
		t.Fatalf("expected node to be created, got nil")
	}
	if node.Id != id {
		t.Errorf("expected node id to be %s, got %s", id, node.Id)
	}
	// ContainerCmd 가 nil 이면 createNodeWithId 를 호출하므로 RunCommand 는 nil 이어야 함.
	if node.RunCommand != nil {
		t.Errorf("expected RunCommand to be nil when ContainerCmd is nil, got %v", node.RunCommand)
	}
	// parentDag 가 올바르게 설정되었는지 확인
	if node.parentDag != dag {
		t.Errorf("expected parentDag to be set to dag, got %v", node.parentDag)
	}
	// dag.nodes 맵에 해당 노드가 등록되었는지 확인
	if dag.nodes[id] != node {
		t.Errorf("expected dag.nodes[%s] to be the created node", id)
	}
	// 같은 id로 다시 노드를 생성하면 nil 이 반환되어야 함.
	if dup := dag.createNode(id); dup != nil {
		t.Errorf("expected duplicate createNode call to return nil, got %v", dup)
	}

	// -------------------------------
	// Case 2: ContainerCmd 가 non-nil 인 경우
	// -------------------------------
	// 새로운 Dag 인스턴스 생성
	dag2 := NewDag()
	// DummyRunnable 을 ContainerCmd 에 할당하여 non-nil 인 경우를 테스트
	dummy := DummyRunnable{}
	dag2.ContainerCmd = dummy

	id2 := "test2"
	node2 := dag2.createNode(id2)
	if node2 == nil {
		t.Fatalf("expected node2 to be created, got nil")
	}
	if node2.Id != id2 {
		t.Errorf("expected node2 id to be %s, got %s", id2, node2.Id)
	}
	// ContainerCmd 가 non-nil 이면 createNode(id, ContainerCmd)를 호출하므로, RunCommand 가 설정되어 있어야 함.
	if node2.RunCommand == nil {
		t.Errorf("expected RunCommand to be set when ContainerCmd is non-nil")
	}
	// 타입 검사를 통해 DummyRunnable 구현체가 맞는지 확인
	if _, ok := node2.RunCommand.(DummyRunnable); !ok {
		t.Errorf("expected RunCommand to be of type DummyRunnable, got %T", node2.RunCommand)
	}
	// 동일한 id로 다시 생성 시 nil을 반환하는지 검증
	if dup2 := dag2.createNode(id2); dup2 != nil {
		t.Errorf("expected duplicate createNode call to return nil, got %v", dup2)
	}
}

func TestCreateEdge(t *testing.T) {
	// 새로운 Dag 인스턴스 생성 (노드와 엣지 초기화)
	dag := &Dag{
		nodes: make(map[string]*Node),
		Edges: []*Edge{},
	}

	// Case 1: parentId가 빈 문자열인 경우 -> Fault 반환
	edge, errType := dag.createEdge("", "child")
	if edge != nil {
		t.Errorf("expected nil edge when parentId is empty, got %+v", edge)
	}
	if errType != Fault {
		t.Errorf("expected error type Fault when parentId is empty, got %v", errType)
	}

	// Case 2: childId가 빈 문자열인 경우 -> Fault 반환
	edge, errType = dag.createEdge("parent", "")
	if edge != nil {
		t.Errorf("expected nil edge when childId is empty, got %+v", edge)
	}
	if errType != Fault {
		t.Errorf("expected error type Fault when childId is empty, got %v", errType)
	}

	// Case 3: 정상적인 엣지 생성
	parentId := "p1"
	childId := "c1"
	edge, errType = dag.createEdge(parentId, childId)
	if edge == nil {
		t.Fatalf("expected edge to be created, got nil")
	}
	if errType != Create {
		t.Errorf("expected error type Create, got %v", errType)
	}
	if edge.parentId != parentId || edge.childId != childId {
		t.Errorf("edge fields mismatch: got parentId=%s, childId=%s; expected parentId=%s, childId=%s",
			edge.parentId, edge.childId, parentId, childId)
	}
	// vertex 채널의 용량(capacity)이 Min과 동일한지 확인
	if cap(edge.vertex) != Min {
		t.Errorf("expected vertex channel capacity to be %d, got %d", Min, cap(edge.vertex))
	}
	// dag.Edges 에 엣지가 추가되었는지 확인
	if len(dag.Edges) != 1 {
		t.Errorf("expected dag.Edges length to be 1, got %d", len(dag.Edges))
	}

	// Case 4: 중복된 엣지 생성 시도 -> nil 과 Exist 에러 반환
	dupEdge, dupErrType := dag.createEdge(parentId, childId)
	if dupEdge != nil {
		t.Errorf("expected duplicate edge creation to return nil, got %+v", dupEdge)
	}
	if dupErrType != Exist {
		t.Errorf("expected error type Exist on duplicate edge creation, got %v", dupErrType)
	}
}

// TestCreateEdge 이게 성공해야지 의미가 있음.
func TestAddEdge(t *testing.T) {
	// 새로운 Dag 인스턴스 생성 (NewDag 사용)
	dag := NewDag()
	if dag == nil {
		t.Fatal("NewDag returned nil")
	}

	// ----- 입력 검증 테스트 -----
	// Case 1: from 와 to가 같은 경우
	err := dag.AddEdge("node1", "node1")
	if err == nil {
		t.Error("expected error when from and to are the same, got nil")
	}

	// Case 2: from 가 빈 문자열인 경우
	err = dag.AddEdge("", "node2")
	if err == nil {
		t.Error("expected error when from is empty, got nil")
	}

	// Case 3: to가 빈 문자열인 경우
	err = dag.AddEdge("node1", "")
	if err == nil {
		t.Error("expected error when to is empty, got nil")
	}

	// ----- 정상적인 엣지 추가 테스트 -----
	// 유효한 입력: "node1" -> "node2"
	err = dag.AddEdge("node1", "node2")
	if err != nil {
		t.Fatalf("unexpected error on valid AddEdge: %v", err)
	}

	// 노드 "node1"와 "node2"가 dag.nodes 에 등록되었는지 확인
	node1, ok := dag.nodes["node1"]
	if !ok || node1 == nil {
		t.Fatal("node1 not found in dag.nodes")
	}
	node2, ok := dag.nodes["node2"]
	if !ok || node2 == nil {
		t.Fatal("node2 not found in dag.nodes")
	}

	// node1의 자식 리스트에 node2가 포함되어 있는지 확인
	found := false
	for _, child := range node1.children {
		if child.Id == "node2" {
			found = true
			break
		}
	}
	if !found {
		t.Error("node2 not found as a child of node1")
	}

	// node2의 부모 리스트에 node1이 포함되어 있는지 확인
	found = false
	for _, parent := range node2.parent {
		if parent.Id == "node1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("node1 not found as a parent of node2")
	}

	// dag.Edges 슬라이스에 엣지가 추가되었는지 확인 (정확히 1개의 엣지)
	if len(dag.Edges) != 1 {
		t.Errorf("expected 1 edge in dag.Edges, got %d", len(dag.Edges))
	}

	// node1의 childrenVertex 와 node2의 parentVertex 가 채워졌는지 확인
	if len(node1.childrenVertex) == 0 {
		t.Error("expected node1.childrenVertex to have at least one channel")
	}
	if len(node2.parentVertex) == 0 {
		t.Error("expected node2.parentVertex to have at least one channel")
	}

	// ----- 중복 엣지 생성 테스트 -----
	// 동일한 from, to 값으로 다시 엣지를 추가하면 오류가 발생해야 함.
	err = dag.AddEdge("node1", "node2")
	if err == nil {
		t.Error("expected error on duplicate edge creation, got nil")
	}
}

// dag.ConnectRunner() 테스트 해야 하는데 여기서 부터는 Node 먼저 테스트 해야함.

// TestCopyDag 는 원본 DAG 에서 사이클 판별에 필요한 최소 정보를 복사하는 copyDagT(apiTest.go 참고) 함수의 동작을 검증
// 이 테스트 케이를 가지고 copyDagT 와 copyDag 를 검사할 수 있음.
func TestCopyDagT(t *testing.T) {
	// 1. 원본 DAG 생성 및 노드/간선 구성
	dag := NewDag()

	// 노드 생성
	nodeA := &Node{Id: "A"}
	nodeB := &Node{Id: "B"}
	nodeC := &Node{Id: "C"}
	nodeD := &Node{Id: "D"}

	// TODO 일단 좀더 복잡한 dag 를 만들어 보는 것도 한번 생각해보자.
	// 부모/자식 관계 설정:
	// A -> B, A -> C, B -> D, C -> D
	nodeA.children = []*Node{nodeB, nodeC}
	nodeB.parent = []*Node{nodeA}
	nodeB.children = []*Node{nodeD}
	nodeC.parent = []*Node{nodeA}
	nodeC.children = []*Node{nodeD}
	nodeD.parent = []*Node{nodeB, nodeC}

	// 원본 DAG 에 노드 등록
	dag.nodes = map[string]*Node{
		nodeA.Id: nodeA,
		nodeB.Id: nodeB,
		nodeC.Id: nodeC,
		nodeD.Id: nodeD,
	}

	// 간선 생성: A->B, A->C, B->D, C->D
	edgeAB := &Edge{parentId: "A", childId: "B"}
	edgeAC := &Edge{parentId: "A", childId: "C"}
	edgeBD := &Edge{parentId: "B", childId: "D"}
	edgeCD := &Edge{parentId: "C", childId: "D"}
	dag.Edges = []*Edge{edgeAB, edgeAC, edgeBD, edgeCD}

	// 2. copyDagT 함수 호출
	newNodes, newEdges := copyDagT(dag)

	// 3. 노드 수 검증
	if len(newNodes) != len(dag.nodes) {
		t.Errorf("expected %d nodes, got %d", len(dag.nodes), len(newNodes))
	}

	// 4. 각 노드의 ID와 부모/자식 관계 검증
	for id, origNode := range dag.nodes {
		newNode, ok := newNodes[id]
		if !ok {
			t.Errorf("node with id %s missing in newNodes", id)
			continue
		}
		// ID 비교
		if newNode.Id != origNode.Id {
			t.Errorf("expected node id %s, got %s", origNode.Id, newNode.Id)
		}
		// 부모 수 비교 및 부모 ID 검증
		if len(newNode.parent) != len(origNode.parent) {
			t.Errorf("node %s: expected %d parents, got %d", id, len(origNode.parent), len(newNode.parent))
		} else {
			for i, p := range newNode.parent {
				if p.Id != origNode.parent[i].Id {
					t.Errorf("node %s: expected parent %s, got %s", id, origNode.parent[i].Id, p.Id)
				}
			}
		}
		// 자식 수 비교 및 자식 ID 검증
		if len(newNode.children) != len(origNode.children) {
			t.Errorf("node %s: expected %d children, got %d", id, len(origNode.children), len(newNode.children))
		} else {
			for i, c := range newNode.children {
				if c.Id != origNode.children[i].Id {
					t.Errorf("node %s: expected child %s, got %s", id, origNode.children[i].Id, c.Id)
				}
			}
		}
	}

	// 5. 간선 복사 검증
	if len(newEdges) != len(dag.Edges) {
		t.Errorf("expected %d edges, got %d", len(dag.Edges), len(newEdges))
	}
	for i, origEdge := range dag.Edges {
		newEdge := newEdges[i]
		if newEdge.parentId != origEdge.parentId || newEdge.childId != origEdge.childId {
			t.Errorf("edge %d: expected parent %s->child %s, got parent %s->child %s",
				i, origEdge.parentId, origEdge.childId, newEdge.parentId, newEdge.childId)
		}
	}

	// 6. (선택사항) 복사본이 독립적인지 검증: 복사본을 수정해도 원본이 변경되지 않아야 함.
	newNodes["A"].Id = "ModifiedA"
	if dag.nodes["A"].Id == "ModifiedA" {
		t.Error("modifying newNodes should not affect original nodes")
	}
}

// TestCopyDags tests both copyDag and copyDagT functions.
func TestCopyDags(t *testing.T) {
	// 1. 원본 DAG 생성 및 노드/간선 구성
	dag := NewDag()

	// 노드 생성
	nodeA := &Node{Id: "A"}
	nodeB := &Node{Id: "B"}
	nodeC := &Node{Id: "C"}
	nodeD := &Node{Id: "D"}

	// 부모/자식 관계 설정:
	// A -> B, A -> C, B -> D, C -> D
	nodeA.children = []*Node{nodeB, nodeC}
	nodeB.parent = []*Node{nodeA}
	nodeB.children = []*Node{nodeD}
	nodeC.parent = []*Node{nodeA}
	nodeC.children = []*Node{nodeD}
	nodeD.parent = []*Node{nodeB, nodeC}

	// 원본 DAG에 노드 등록
	dag.nodes = map[string]*Node{
		nodeA.Id: nodeA,
		nodeB.Id: nodeB,
		nodeC.Id: nodeC,
		nodeD.Id: nodeD,
	}

	// 간선 생성: A->B, A->C, B->D, C->D
	edgeAB := &Edge{parentId: "A", childId: "B"}
	edgeAC := &Edge{parentId: "A", childId: "C"}
	edgeBD := &Edge{parentId: "B", childId: "D"}
	edgeCD := &Edge{parentId: "C", childId: "D"}
	dag.Edges = []*Edge{edgeAB, edgeAC, edgeBD, edgeCD}

	// 2. 복사된 DAG 의 구조를 검증하는 헬퍼 함수 정의
	verifyCopiedDag := func(newNodes map[string]*Node, newEdges []*Edge, methodName string) {
		// 노드 수 검증
		if len(newNodes) != len(dag.nodes) {
			t.Errorf("[%s] expected %d nodes, got %d", methodName, len(dag.nodes), len(newNodes))
		}

		// 각 노드의 ID와 부모/자식 관계 검증
		for id, origNode := range dag.nodes {
			newNode, ok := newNodes[id]
			if !ok {
				t.Errorf("[%s] node with id %s missing in newNodes", methodName, id)
				continue
			}
			// ID 비교
			if newNode.Id != origNode.Id {
				t.Errorf("[%s] expected node id %s, got %s", methodName, origNode.Id, newNode.Id)
			}
			// 부모 수 비교 및 부모 ID 검증
			if len(newNode.parent) != len(origNode.parent) {
				t.Errorf("[%s] node %s: expected %d parents, got %d", methodName, id, len(origNode.parent), len(newNode.parent))
			} else {
				for i, p := range newNode.parent {
					if p.Id != origNode.parent[i].Id {
						t.Errorf("[%s] node %s: expected parent %s, got %s", methodName, id, origNode.parent[i].Id, p.Id)
					}
				}
			}
			// 자식 수 비교 및 자식 ID 검증
			if len(newNode.children) != len(origNode.children) {
				t.Errorf("[%s] node %s: expected %d children, got %d", methodName, id, len(origNode.children), len(newNode.children))
			} else {
				for i, c := range newNode.children {
					if c.Id != origNode.children[i].Id {
						t.Errorf("[%s] node %s: expected child %s, got %s", methodName, id, origNode.children[i].Id, c.Id)
					}
				}
			}
		}

		// 간선 복사 검증
		if len(newEdges) != len(dag.Edges) {
			t.Errorf("[%s] expected %d edges, got %d", methodName, len(dag.Edges), len(newEdges))
		}
		for i, origEdge := range dag.Edges {
			newEdge := newEdges[i]
			if newEdge.parentId != origEdge.parentId || newEdge.childId != origEdge.childId {
				t.Errorf("[%s] edge %d: expected parent %s->child %s, got parent %s->child %s",
					methodName, i, origEdge.parentId, origEdge.childId, newEdge.parentId, newEdge.childId)
			}
		}

		// 복사본이 독립적인지 검증: 복사본을 수정해도 원본이 변경되지 않아야 함.
		originalAId := dag.nodes["A"].Id
		newNodes["A"].Id = "ModifiedA"
		if dag.nodes["A"].Id == "ModifiedA" {
			t.Errorf("[%s] modifying newNodes should not affect original nodes", methodName)
		}
		// 원본 노드의 ID 복원
		dag.nodes["A"].Id = originalAId
	}

	// 3. copyDag 함수 테스트
	newNodes1, newEdges1 := copyDag(dag)
	verifyCopiedDag(newNodes1, newEdges1, "copyDag")

	// 4. copyDagT 함수 테스트
	newNodes2, newEdges2 := copyDagT(dag)
	verifyCopiedDag(newNodes2, newEdges2, "copyDagT")
}

func TestSimpleDag(t *testing.T) {
	// runnable := Connect()
	dag, _ := InitDag()
	// dag.SetContainerCmd(runnable)

	// 엣지 추가
	if err := dag.AddEdge(dag.StartNode.Id, "1"); err != nil {
		t.Fatalf("failed to add edge from StartNode to '1': %v", err)
	}
	if err := dag.AddEdge("1", "2"); err != nil {
		t.Fatalf("failed to add edge from '1' to '2': %v", err)
	}
	if err := dag.AddEdge("1", "3"); err != nil {
		t.Fatalf("failed to add edge from '1' to '3': %v", err)
	}
	if err := dag.AddEdge("1", "4"); err != nil {
		t.Fatalf("failed to add edge from '1' to '4': %v", err)
	}
	if err := dag.AddEdge("2", "5"); err != nil {
		t.Fatalf("failed to add edge from '2' to '5': %v", err)
	}
	if err := dag.AddEdge("5", "6"); err != nil {
		t.Fatalf("failed to add edge from '5' to '6': %v", err)
	}

	// DAG 완성 처리
	if err := dag.FinishDag(); err != nil {
		t.Fatalf("FinishDag failed: %v", err)
	}

	ctx := context.Background()
	dag.ConnectRunner()
	dag.GetReady(ctx)
	b1 := dag.Start()
	if b1 != true {
		t.Errorf("expected Start() to return true, got %v", b1)
	}

	b2 := dag.Wait(ctx)
	if b2 != true {
		t.Errorf("expected Wait() to return true, got %v", b2)
	}
}

func TestDetectCycle(t *testing.T) {
	// 새로운 DAG 생성
	dag, _ := InitDag()

	// 엣지 추가
	if err := dag.AddEdge(dag.StartNode.Id, "1"); err != nil {
		t.Fatalf("failed to add edge from StartNode to '1': %v", err)
	}
	if err := dag.AddEdge("1", "2"); err != nil {
		t.Fatalf("failed to add edge from '1' to '2': %v", err)
	}
	if err := dag.AddEdge("1", "3"); err != nil {
		t.Fatalf("failed to add edge from '1' to '3': %v", err)
	}
	if err := dag.AddEdge("1", "4"); err != nil {
		t.Fatalf("failed to add edge from '1' to '4': %v", err)
	}
	if err := dag.AddEdge("2", "5"); err != nil {
		t.Fatalf("failed to add edge from '2' to '5': %v", err)
	}
	if err := dag.AddEdge("5", "6"); err != nil {
		t.Fatalf("failed to add edge from '5' to '6': %v", err)
	}

	// DAG 완성 처리
	if err := dag.FinishDag(); err != nil {
		t.Fatalf("FinishDag failed: %v", err)
	}

	// 방문 기록 맵 초기화
	visit := make(map[string]bool)
	for id := range dag.nodes {
		visit[id] = false
	}

	// detectCycle 함수 테스트
	cycle, end := dag.detectCycle(dag.StartNode.Id, dag.StartNode.Id, visit)
	if cycle {
		t.Errorf("expected no cycle, but detected one")
	}
	if !end {
		t.Errorf("expected the entire graph to be traversed, but it was not")
	}
}
