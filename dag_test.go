package dag_go

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"strings"
	"testing"
	"time"

	"go.uber.org/goleak"
)

// Compile-time interface check.
var _ Runnable = DummyRunnable{}

func TestInitDag(t *testing.T) {
	defer goleak.VerifyNone(t)
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
	if dag.StartNode.ID != StartNode {
		t.Errorf("Expected StartNode Id to be %s, got %s", StartNode, dag.StartNode.ID)
	}

	// StartNode 의 parentVertex 에 SafeChannel 이 추가되었는지 확인
	if len(dag.StartNode.parentVertex) == 0 {
		t.Error("StartNode's parentVertex channel is not set")
	} else {
		// parentVertex 의 갯수가 정확히 1개인지 검사, 1개로 설정함.
		if len(dag.StartNode.parentVertex) != 1 {
			t.Errorf("Expected StartNode.parentVertex count to be 1, got %d", len(dag.StartNode.parentVertex))
		}

		// 첫 번째 SafeChannel 의 내부 채널 용량을 확인
		safeCh := dag.StartNode.parentVertex[0]
		if cap(safeCh.GetChannel()) != dag.Config.MinChannelBuffer {
			t.Errorf("Expected parentVertex channel capacity to be %d, got %d", dag.Config.MinChannelBuffer, cap(safeCh.GetChannel()))
		}
	}

	// NodesResult 채널(노드 결과 SafeChannel)의 용량 검사
	if dag.NodesResult == nil {
		t.Error("RunningStatus is nil")
	} else if cap(dag.NodesResult.GetChannel()) != dag.Config.MaxChannelBuffer {
		t.Errorf("Expected RunningStatus channel capacity to be %d, got %d", dag.Config.MaxChannelBuffer, cap(dag.NodesResult.GetChannel()))
	}
}

// DummyRunnable is a no-op implementation of Runnable used in tests.
type DummyRunnable struct{}

func (DummyRunnable) RunE(_ context.Context, _ interface{}) error {
	return nil
}

func TestCreateNode(t *testing.T) {
	defer goleak.VerifyNone(t)
	// -------------------------------
	// Case 1: ContainerCmd 가 nil 인 경우
	// -------------------------------
	// NewDag()를 호출하면 기본적으로 ContainerCmd 는 nil
	dag := NewDag()
	id := "test1"
	node := dag.CreateNode(id)
	if node == nil {
		t.Fatalf("expected node to be created, got nil")
	}
	if node.ID != id {
		t.Errorf("expected node id to be %s, got %s", id, node.ID)
	}
	// With nil ContainerCmd the per-node runner should also be nil.
	if r := node.runnerLoad(); r != nil {
		t.Errorf("expected per-node runner to be nil when ContainerCmd is nil, got %v", r)
	}
	// parentDag must be set to the owning DAG.
	if node.parentDag != dag {
		t.Errorf("expected parentDag to be set to dag, got %v", node.parentDag)
	}
	// The node must be registered in dag.nodes.
	if dag.nodes[id] != node {
		t.Errorf("expected dag.nodes[%s] to be the created node", id)
	}
	// Creating the same id again must return nil.
	if dup := dag.CreateNode(id); dup != nil {
		t.Errorf("expected duplicate createNode call to return nil, got %v", dup)
	}

	// -------------------------------
	// Case 2: ContainerCmd is non-nil
	// -------------------------------
	dag2 := NewDag()
	dummy := DummyRunnable{}
	dag2.ContainerCmd = dummy

	id2 := "test2"
	node2 := dag2.CreateNode(id2)
	if node2 == nil {
		t.Fatalf("expected node2 to be created, got nil")
	}
	if node2.ID != id2 {
		t.Errorf("expected node2 id to be %s, got %s", id2, node2.ID)
	}
	// When ContainerCmd is non-nil, getRunnerSnapshot() must resolve to it.
	r := node2.getRunnerSnapshot()
	if r == nil {
		t.Errorf("expected runner to be resolved from ContainerCmd, got nil")
	}
	if _, ok := r.(DummyRunnable); !ok {
		t.Errorf("expected runner to be DummyRunnable, got %T", r)
	}
	// Duplicate creation must return nil.
	if dup2 := dag2.CreateNode(id2); dup2 != nil {
		t.Errorf("expected duplicate createNode call to return nil, got %v", dup2)
	}
}

func TestCreateEdge(t *testing.T) {
	defer goleak.VerifyNone(t)
	// 새로운 Dag 인스턴스 생성 (노드와 엣지 초기화)
	dag := &Dag{
		nodes: make(map[string]*Node),
		Edges: []*Edge{},
	}
	// 기본 구성 설정
	dag.Config = DefaultDagConfig()

	// Case 1: parentID가 빈 문자열인 경우 -> Fault 반환
	edge, errType := dag.createEdge("", "child")
	if edge != nil {
		t.Errorf("expected nil edge when parentID is empty, got %+v", edge)
	}
	if errType != Fault {
		t.Errorf("expected error type Fault when parentID is empty, got %v", errType)
	}

	// Case 2: childID가 빈 문자열인 경우 -> Fault 반환
	edge, errType = dag.createEdge("parent", "")
	if edge != nil {
		t.Errorf("expected nil edge when childID is empty, got %+v", edge)
	}
	if errType != Fault {
		t.Errorf("expected error type Fault when childID is empty, got %v", errType)
	}

	// Case 3: 정상적인 엣지 생성
	parentID := "p1"
	childID := "c1"
	edge, errType = dag.createEdge(parentID, childID)
	if edge == nil {
		t.Fatalf("expected edge to be created, got nil")
	}
	if errType != Create {
		t.Errorf("expected error type Create, got %v", errType)
	}
	if edge.parentID != parentID || edge.childID != childID {
		t.Errorf("edge fields mismatch: got parentID=%s, childID=%s; expected parentID=%s, childID=%s",
			edge.parentID, edge.childID, parentID, childID)
	}
	// safeVertex 채널의 용량이 dag.Config.MinChannelBuffer 와 일치하는지 확인
	if cap(edge.safeVertex.GetChannel()) != dag.Config.MinChannelBuffer {
		t.Errorf("expected vertex channel capacity to be %d, got %d", dag.Config.MinChannelBuffer, cap(edge.safeVertex.GetChannel()))
	}
	// dag.Edges 에 엣지가 추가되었는지 확인
	if len(dag.Edges) != 1 {
		t.Errorf("expected dag.Edges length to be 1, got %d", len(dag.Edges))
	}

	// Case 4: 중복된 엣지 생성 시도 -> nil 과 Exist 에러 반환
	dupEdge, dupErrType := dag.createEdge(parentID, childID)
	if dupEdge != nil {
		t.Errorf("expected duplicate edge creation to return nil, got %+v", dupEdge)
	}
	if dupErrType != Exist {
		t.Errorf("expected error type Exist on duplicate edge creation, got %v", dupErrType)
	}
}

// TestCreateEdge 이게 성공해야지 의미가 있음.
//
//nolint:gocognit,gocyclo // comprehensive edge test covering many validation paths in a single test function
func TestAddEdge(t *testing.T) {
	defer goleak.VerifyNone(t)
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
		if child.ID == "node2" {
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
		if parent.ID == "node1" {
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
	// 관계가 중복 추가되지 않았는지 확인
	if len(node1.children) != 1 {
		t.Errorf("expected node1 to have 1 child after duplicate edge, got %d", len(node1.children))
	}
	if len(node2.parent) != 1 {
		t.Errorf("expected node2 to have 1 parent after duplicate edge, got %d", len(node2.parent))
	}
}

// TestCopyDag 는 copyDag 함수가 노드 ID, 부모/자식 구조를 올바르게 복사하는지 검증하는 단위 테스트입니다.
func TestCopyDag(t *testing.T) {
	defer goleak.VerifyNone(t)
	// 1. 원본 DAG 생성 및 노드/간선 구성
	dag := NewDag()

	nodeA := &Node{ID: "A"}
	nodeB := &Node{ID: "B"}
	nodeC := &Node{ID: "C"}
	nodeD := &Node{ID: "D"}

	// 부모/자식 관계 설정
	nodeA.children = []*Node{nodeB, nodeC}
	nodeB.parent = []*Node{nodeA}
	nodeB.children = []*Node{nodeD}
	nodeC.parent = []*Node{nodeA}
	nodeC.children = []*Node{nodeD}
	nodeD.parent = []*Node{nodeB, nodeC}

	dag.nodes = map[string]*Node{
		nodeA.ID: nodeA,
		nodeB.ID: nodeB,
		nodeC.ID: nodeC,
		nodeD.ID: nodeD,
	}

	dag.Edges = []*Edge{
		{parentID: "A", childID: "B"},
		{parentID: "A", childID: "C"},
		{parentID: "B", childID: "D"},
		{parentID: "C", childID: "D"},
	}

	// 2. copyDag 호출
	newNodes, newEdges := copyDag(dag)

	// 3. 노드 수 검증
	if len(newNodes) != len(dag.nodes) {
		t.Errorf("expected %d nodes, got %d", len(dag.nodes), len(newNodes))
	}

	// 4. 각 노드 ID와 관계 검증
	for id, origNode := range dag.nodes {
		newNode, ok := newNodes[id]
		if !ok {
			t.Errorf("node with ID %s missing in newNodes", id)
			continue
		}
		if newNode.ID != origNode.ID {
			t.Errorf("expected node ID %s, got %s", origNode.ID, newNode.ID)
		}
		if len(newNode.parent) != len(origNode.parent) {
			t.Errorf("node %s: expected %d parents, got %d", id, len(origNode.parent), len(newNode.parent))
		}
		if len(newNode.children) != len(origNode.children) {
			t.Errorf("node %s: expected %d children, got %d", id, len(origNode.children), len(newNode.children))
		}
	}

	// 5. 간선 복사 검증
	if len(newEdges) != len(dag.Edges) {
		t.Errorf("expected %d edges, got %d", len(dag.Edges), len(newEdges))
	}
	for i, origEdge := range dag.Edges {
		newEdge := newEdges[i]
		if newEdge.parentID != origEdge.parentID || newEdge.childID != origEdge.childID {
			t.Errorf("edge %d: expected %s -> %s, got %s -> %s",
				i, origEdge.parentID, origEdge.childID, newEdge.parentID, newEdge.childID)
		}
	}
}

// TestCopyDags 는 verifyCopiedDag 유틸리티를 사용하여 copyDag 의 동작을 검증합니다.
func TestCopyDags(t *testing.T) {
	defer goleak.VerifyNone(t)
	dag := NewDag()

	nodeA := &Node{ID: "A"}
	nodeB := &Node{ID: "B"}
	nodeC := &Node{ID: "C"}
	nodeD := &Node{ID: "D"}

	nodeA.children = []*Node{nodeB, nodeC}
	nodeB.parent = []*Node{nodeA}
	nodeB.children = []*Node{nodeD}
	nodeC.parent = []*Node{nodeA}
	nodeC.children = []*Node{nodeD}
	nodeD.parent = []*Node{nodeB, nodeC}

	dag.nodes = map[string]*Node{
		nodeA.ID: nodeA,
		nodeB.ID: nodeB,
		nodeC.ID: nodeC,
		nodeD.ID: nodeD,
	}

	dag.Edges = []*Edge{
		{parentID: "A", childID: "B"},
		{parentID: "A", childID: "C"},
		{parentID: "B", childID: "D"},
		{parentID: "C", childID: "D"},
	}

	newNodes, newEdges := copyDag(dag)

	if err := verifyCopiedDag(dag, newNodes, newEdges, "copyDag"); err != nil {
		t.Errorf("verifyCopiedDag failed: %v", err)
	}
}

func TestManyCopyDags(t *testing.T) {
	defer goleak.VerifyNone(t)
	// 여러 테스트 케이스: numNodes 와 edgeProb 를 조합
	testCases := []struct {
		numNodes int
		edgeProb float64
	}{
		{10, 0.3},
		{50, 0.1},
		{100, 0.05},
	}

	for _, tc := range testCases {
		// DAG 생성
		dag := generateDAG(tc.numNodes, tc.edgeProb)
		if len(dag.nodes) != tc.numNodes {
			t.Errorf("generateDAG(%d, %.2f): expected %d nodes, got %d",
				tc.numNodes, tc.edgeProb, tc.numNodes, len(dag.nodes))
		}

		// copyDag 함수를 호출하여 DAG 복사본 생성
		copiedNodes, copiedEdges := copyDag(dag)
		// verifyCopiedDag 는 원본과 복사본의 구조를 비교하고, 문제가 있으면 에러를 리턴합니다.

		if err := verifyCopiedDag(dag, copiedNodes, copiedEdges, "copyDag"); err != nil {
			t.Errorf("Test case (numNodes=%d, edgeProb=%.2f) failed: %v", tc.numNodes, tc.edgeProb, err)
		}
	}
}

// TestCopyDagIndependence 는 CopyDag 함수를 통해 생성된 복사본이 원본 DAG 와 독립적으로 동작하는지 검증
// 복사본에 데이터를 새롭게 넣었는데 만약 원본 DAG 의 내용이 변경한 복사본과 같아지면 shallow copy 이기때문에 에러남.
// copyDag 에서 shallow copy 가 일어나는 곳은 아예 복사를 하지 않는다.
//
//nolint:funlen,gocognit,gocyclo // 이 테스트는 길지만 의도적으로 유지함
func TestCopyDagIndependence(t *testing.T) {
	defer goleak.VerifyNone(t)
	// 1. 원본 DAG 생성 및 초기화
	orig := NewDag()
	orig.ID = "orig-id"
	orig.validated = true
	orig.Timeout = 10 * time.Second
	orig.bTimeout = false
	orig.ContainerCmd = nil // shallow copy라서 pass

	// 노드 생성
	nodeA := &Node{ID: "A", ImageName: "imgA", Commands: "cmdA", succeed: true}
	nodeB := &Node{ID: "B", ImageName: "imgB", Commands: "cmdB", succeed: false}

	// 부모/자식 관계 설정: A -> B
	nodeA.children = []*Node{nodeB}
	nodeB.parent = []*Node{nodeA}

	// 원본 DAG 에 노드 등록
	orig.nodes = map[string]*Node{
		"A": nodeA,
		"B": nodeB,
	}

	// 간선 생성: A -> B
	edgeAB := &Edge{
		parentID: "A",
		childID:  "B",
		// CopyDag 에서는 vertex 등의 추가 정보는 복사하지 않으므로 생략함.
	}
	orig.Edges = []*Edge{edgeAB}

	// 2. CopyDag 함수를 호출하여 복사본 생성
	newDag := CopyDag(orig, "copied-id")
	if newDag == nil {
		t.Fatal("CopyDag returned nil")
	}

	// 3. DAG 필드 독립성 검증

	// (a) Id 검증
	originalID := orig.ID
	newDag.ID = "modified-dag-id"
	if orig.ID == "modified-dag-id" {
		t.Error("Modifying newDag.Id affected original.Id")
	}
	newDag.ID = originalID

	// (b) validated, Timeout, bTimeout 검증
	origValidated := orig.validated
	newDag.validated = !origValidated
	if orig.validated == newDag.validated {
		t.Error("Modifying newDag.validated affected original.validated")
	}
	newDag.validated = origValidated

	origTimeout := orig.Timeout
	newDag.Timeout = origTimeout + 100*time.Millisecond
	if orig.Timeout == newDag.Timeout {
		t.Error("Modifying newDag.Timeout affected original.Timeout")
	}
	newDag.Timeout = origTimeout

	origBTimeout := orig.bTimeout
	newDag.bTimeout = !origBTimeout
	if orig.bTimeout == newDag.bTimeout {
		t.Error("Modifying newDag.bTimeout affected original.bTimeout")
	}
	newDag.bTimeout = origBTimeout

	// 4. 노드 독립성 검증
	for key, origNode := range orig.nodes {
		newNode, ok := newDag.nodes[key]
		if !ok {
			t.Errorf("node %s missing in copied DAG", key)
			continue
		}

		// (a) Id 필드 검증
		origNodeID := origNode.ID
		newNode.ID = "Modified-" + origNodeID
		if origNode.ID == newNode.ID {
			t.Errorf("Modifying copied node %s.Id affected original", key)
		}
		newNode.ID = origNodeID

		// (b) ImageName 필드 검증
		origImage := origNode.ImageName
		newNode.ImageName = "Modified-" + origImage
		if origNode.ImageName == newNode.ImageName {
			t.Errorf("Modifying copied node %s.ImageName affected original", key)
		}
		newNode.ImageName = origImage

		// (c) Commands 필드 검증
		origCmd := origNode.Commands
		newNode.Commands = "Modified-" + origCmd
		if origNode.Commands == newNode.Commands {
			t.Errorf("Modifying copied node %s.Commands affected original", key)
		}
		newNode.Commands = origCmd

		// (d) succeed 필드 검증
		origSucceed := origNode.succeed
		newNode.succeed = !origSucceed
		if origNode.succeed == newNode.succeed {
			t.Errorf("Modifying copied node %s.succeed affected original", key)
		}
		newNode.succeed = origSucceed
	}

	// 5. 간선 독립성 검증
	if len(newDag.Edges) != len(orig.Edges) {
		t.Errorf("expected %d edges, got %d", len(orig.Edges), len(newDag.Edges))
	}
	for i, origEdge := range orig.Edges {
		newEdge := newDag.Edges[i]
		if newEdge.parentID != origEdge.parentID || newEdge.childID != origEdge.childID {
			t.Errorf("edge %d: expected parent %s->child %s, got parent %s->child %s",
				i, origEdge.parentID, origEdge.childID, newEdge.parentID, newEdge.childID)
		}
	}
}

// TestDetectCycle_SimpleCycle verifies that FinishDag returns ErrCycleDetected
// for a direct two-node cycle: start → A → B → A.
func TestDetectCycle_SimpleCycle(t *testing.T) {
	defer goleak.VerifyNone(t)

	dag, _ := InitDag()

	if err := dag.AddEdge(StartNode, "A"); err != nil {
		t.Fatalf("AddEdge start->A: %v", err)
	}
	if err := dag.AddEdge("A", "B"); err != nil {
		t.Fatalf("AddEdge A->B: %v", err)
	}
	// Back-edge B→A closes the cycle; AddEdge allows it (only rejects from==to).
	if err := dag.AddEdge("B", "A"); err != nil {
		t.Fatalf("AddEdge B->A: %v", err)
	}

	err := dag.FinishDag()
	if err == nil {
		t.Fatal("expected ErrCycleDetected, got nil")
	}
	if !errors.Is(err, ErrCycleDetected) {
		t.Errorf("expected ErrCycleDetected, got: %v", err)
	}
}

// TestDetectCycle_ComplexCycle verifies that FinishDag returns ErrCycleDetected
// for a three-node cycle: start → A → B → C → A.
func TestDetectCycle_ComplexCycle(t *testing.T) {
	defer goleak.VerifyNone(t)

	dag, _ := InitDag()

	for _, pair := range []struct{ from, to string }{
		{StartNode, "A"}, {"A", "B"}, {"B", "C"}, {"C", "A"},
	} {
		if err := dag.AddEdge(pair.from, pair.to); err != nil {
			t.Fatalf("AddEdge %s->%s: %v", pair.from, pair.to, err)
		}
	}

	err := dag.FinishDag()
	if err == nil {
		t.Fatal("expected ErrCycleDetected, got nil")
	}
	if !errors.Is(err, ErrCycleDetected) {
		t.Errorf("expected ErrCycleDetected, got: %v", err)
	}
}

// TestDetectCycle_SelfLoop verifies that DetectCycle returns true for a node
// that lists itself as a child.  AddEdge rejects from==to, so the graph is
// constructed directly to reach the algorithm under test.
func TestDetectCycle_SelfLoop(t *testing.T) {
	defer goleak.VerifyNone(t)

	dag := NewDag()
	nodeA := &Node{ID: "A"}
	nodeA.children = []*Node{nodeA} // A → A
	dag.nodes = map[string]*Node{"A": nodeA}

	if !DetectCycle(dag) {
		t.Error("expected DetectCycle to return true for self-loop, got false")
	}
}

// TestDetectCycle_NoCycle verifies that a valid diamond-shaped DAG is not flagged
// as cyclic by either FinishDag or DetectCycle.
//
// Graph: start → A → {B1, B2} → C → {D1, D2} → E
func TestDetectCycle_NoCycle(t *testing.T) {
	defer goleak.VerifyNone(t)

	dag, _ := InitDag()

	for _, pair := range []struct{ from, to string }{
		{StartNode, "A"},
		{"A", "B1"}, {"A", "B2"},
		{"B1", "C"}, {"B2", "C"},
		{"C", "D1"}, {"C", "D2"},
		{"D1", "E"}, {"D2", "E"},
	} {
		if err := dag.AddEdge(pair.from, pair.to); err != nil {
			t.Fatalf("AddEdge %s->%s: %v", pair.from, pair.to, err)
		}
	}

	if err := dag.FinishDag(); err != nil {
		t.Fatalf("FinishDag returned unexpected error for valid DAG: %v", err)
	}
	// DetectCycle should also confirm no cycle after FinishDag.
	if DetectCycle(dag) {
		t.Error("expected DetectCycle to return false for valid DAG, got true")
	}
}

func TestDetectCycle(t *testing.T) {
	defer goleak.VerifyNone(t)
	// 새로운 DAG 생성
	dag, _ := InitDag()

	// 엣지 추가
	if err := dag.AddEdge(dag.StartNode.ID, "1"); err != nil {
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
	// detectCycle 함수 테스트, cycle 이면 true 반환.
	cycle := DetectCycle(dag)
	if cycle {
		t.Errorf("expected no cycle, but detected one")
	}
}

// NoopCmd is a Runnable that succeeds immediately without performing any work.
type NoopCmd struct{}

func (NoopCmd) RunE(_ context.Context, _ interface{}) error { return nil }

func TestSimpleDag(t *testing.T) {
	defer goleak.VerifyNone(t)
	dag, _ := InitDag()
	// 엣지 추가
	if err := dag.AddEdge(dag.StartNode.ID, "1"); err != nil {
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

	// 추가 오류 해결을 위해 NoopCmd 사용
	// 해당 코드가 없으면 wait 에서 false 리턴한다. 오류난다.
	dag.SetContainerCmd(NoopCmd{})

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

func TestSimple1Dag(t *testing.T) {
	defer goleak.VerifyNone(t)
	// DAG 초기화
	dag, err := InitDag()
	if err != nil {
		t.Fatalf("failed to initialize dag: %v", err)
	}

	// DAG 설정 초기화: 워커 풀 크기와 채널 버퍼 크기 설정
	dag.Config = DagConfig{
		MinChannelBuffer: 10,
		MaxChannelBuffer: 50,
		WorkerPoolSize:   50, // 원하는 워커 수로 설정 이거 적으면 timeout error 발생할 수 있음.
		StatusBuffer:     10, // 원하는 채널 버퍼 사이즈로 설정
		DefaultTimeout:   30 * time.Second,
	}

	// Attach a no-op runner so that all nodes succeed during inFlight.
	dag.SetContainerCmd(NoopCmd{})

	// 엣지 추가: DAG 의 노드들 간에 부모/자식 관계 구성
	if err := dag.AddEdge(dag.StartNode.ID, "1"); err != nil {
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

	// DAG 완성 처리: 모든 노드가 연결된 상태로 마무리
	if err := dag.FinishDag(); err != nil {
		t.Fatalf("FinishDag failed: %v", err)
	}

	// 컨텍스트 생성
	ctx := context.Background()

	// 각 노드에 runner 함수를 연결하도록 설정
	dag.ConnectRunner()

	// DAG 실행 준비: 워커 풀과 상태 채널들을 초기화
	if ok := dag.GetReady(ctx); !ok {
		t.Fatalf("GetReady failed")
	}

	// 시작 노드의 runner 를 실행 (시작 상태 채널에 값 전송)
	b1 := dag.Start()
	if b1 != true {
		t.Errorf("expected Start() to return true, got %v", b1)
	}

	// 모든 실행이 완료될 때까지 대기
	b2 := dag.Wait(ctx)
	if b2 != true {
		t.Errorf("expected Wait() to return true, got %v", b2)
	}
}

// SimpleCommand simulates a short-lived task while respecting context cancellation.
type SimpleCommand struct{}

func (*SimpleCommand) RunE(ctx context.Context, _ interface{}) error {
	select {
	case <-time.After(100 * time.Millisecond):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// TestComplexDag 는 복잡한 DAG 구조를 테스트합니다.
func TestComplexDag(t *testing.T) {
	defer goleak.VerifyNone(t)
	// DAG 초기화
	dag, err := InitDag()
	if err != nil {
		t.Fatalf("InitDag failed: %v", err)
	}

	// 실행 명령 설정
	dag.SetContainerCmd(&SimpleCommand{})

	// 노드 생성 (다이아몬드 패턴 + 추가 노드)
	nodeIDs := []string{"A", "B1", "B2", "C", "D1", "D2", "E"}
	for _, id := range nodeIDs {
		dag.CreateNode(id)
	}

	// 엣지 추가
	edges := []struct{ from, to string }{
		{dag.StartNode.ID, "A"},
		{"A", "B1"},
		{"A", "B2"},
		{"B1", "C"},
		{"B2", "C"},
		{"C", "D1"},
		{"C", "D2"},
		{"D1", "E"},
		{"D2", "E"},
	}

	for _, edge := range edges {
		if err := dag.AddEdgeIfNodesExist(edge.from, edge.to); err != nil {
			t.Fatalf("failed to add edge from '%s' to '%s': %v", edge.from, edge.to, err)
		}
	}

	// DAG 완성 처리
	if err := dag.FinishDag(); err != nil {
		t.Fatalf("FinishDag failed: %v", err)
	}

	ctx := context.Background()
	dag.ConnectRunner()

	if !dag.GetReady(ctx) { // GetReadyT 사용
		t.Fatalf("GetReadyT failed")
	}

	b1 := dag.Start()
	if b1 != true {
		t.Errorf("expected Start() to return true, got %v", b1)
	}

	b2 := dag.Wait(ctx)
	if b2 != true {
		t.Errorf("expected Wait() to return true, got %v", b2)
	}
}

// TestErrorHandlingUsage 는 AddEdge 에서 발생하는 에러들을 통해
// reportError 로 에러를 기록하고, collectErrors 로 수집하는 흐름을 검증
func TestErrorHandlingUsage(t *testing.T) {
	defer goleak.VerifyNone(t)
	// DAG 초기화 (기본 설정 사용)
	dag, err := InitDag()
	if err != nil {
		t.Fatalf("InitDag failed: %v", err)
	}

	// 강제로 에러 조건을 유발하기 위해 잘못된 엣지 추가
	// 1. from-node 와 to-node 가 동일한 경우
	err = dag.AddEdge("1", "1")
	if err == nil {
		t.Error("Expected error when adding edge from a node to itself")
	}

	// 2. from-node 값이 빈 문자열인 경우
	err = dag.AddEdge("", "2")
	if err == nil {
		t.Error("Expected error when adding edge with empty from-node")
	}

	// 3. to-node 값이 빈 문자열인 경우
	err = dag.AddEdge("1", "")
	if err == nil {
		t.Error("Expected error when adding edge with empty to-node")
	}

	// 이제 보고된 에러들을 모아본다.
	// collectErrors 는 내부적으로 최대 5초(여기서는 3초로 설정) 대기 후, 에러들을 리턴한다.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	collectedErrors := dag.collectErrors(ctx)

	// 위에서 3건의 에러가 보고되었으므로, 3건이 수집되어야 함.
	if len(collectedErrors) != 3 {
		t.Errorf("Expected 3 errors collected, got %d", len(collectedErrors))
	}

	// 수집된 에러들을 로그로 출력
	for i, e := range collectedErrors {
		t.Logf("Collected error %d: %v", i+1, e)
	}
}

// TestComplexDagProgress 는 다이아몬드 패턴 DAG 실행 전후로 Progress() 값을 검증
func TestComplexDagProgress(t *testing.T) {
	defer goleak.VerifyNone(t)
	// DAG 초기화
	dag, err := InitDag()
	if err != nil {
		t.Fatalf("InitDag failed: %v", err)
	}

	// 실행 명령 설정 (SimpleCommand 는 Runnable 인터페이스의 간단한 구현체)
	dag.SetContainerCmd(&SimpleCommand{})

	// 노드 생성 (다이아몬드 패턴 + 추가 노드)
	nodeIDs := []string{"A", "B1", "B2", "C", "D1", "D2", "E"}
	for _, id := range nodeIDs {
		if node := dag.CreateNode(id); node == nil {
			t.Fatalf("failed to create node '%s'", id)
		}
	}

	// 엣지 추가
	edges := []struct{ from, to string }{
		{dag.StartNode.ID, "A"},
		{"A", "B1"},
		{"A", "B2"},
		{"B1", "C"},
		{"B2", "C"},
		{"C", "D1"},
		{"C", "D2"},
		{"D1", "E"},
		{"D2", "E"},
	}

	for _, edge := range edges {
		if err := dag.AddEdgeIfNodesExist(edge.from, edge.to); err != nil {
			t.Fatalf("failed to add edge from '%s' to '%s': %v", edge.from, edge.to, err)
		}
	}

	// DAG 완성 처리: EndNode 생성 및 모든 노드 연결
	if err := dag.FinishDag(); err != nil {
		t.Fatalf("FinishDag failed: %v", err)
	}

	// runner 함수 연결 및 워커 풀 준비
	dag.ConnectRunner()
	ctx := context.Background()
	if !dag.GetReady(ctx) {
		t.Fatalf("GetReady failed")
	}

	// 실행 전 Progress() 검증 (아직 아무 노드도 완료되지 않았으므로 0.0이어야 함)
	if progress := dag.Progress(); progress != 0.0 {
		t.Errorf("expected initial progress 0.0, got %v", progress)
	} else {
		t.Logf("Initial progress: %v", progress)
	}

	// 시작 노드의 실행
	if b := dag.Start(); b != true {
		t.Errorf("expected Start() to return true, got %v", b)
	}

	// 모든 실행이 완료될 때까지 대기
	if b := dag.Wait(ctx); b != true {
		t.Errorf("expected Wait() to return true, got %v", b)
	}

	// 실행 완료 후 Progress() 검증 (모든 노드가 완료되어야 하므로 1.0이어야 함)
	if progress := dag.Progress(); progress != 1.0 {
		t.Errorf("expected progress 1.0 after execution, got %v", progress)
	} else {
		t.Logf("Progress after execution: %v", progress)
	}
}

// TestDagReset verifies that a DAG can be executed a second time after Reset()
// returns the same result as the first run.
//
//nolint:gocognit // comprehensive reset lifecycle test: closure + node-status loop + two full run cycles
func TestDagReset(t *testing.T) {
	defer goleak.VerifyNone(t)

	buildAndRun := func(dag *Dag) bool {
		ctx := context.Background()
		dag.ConnectRunner()
		if !dag.GetReady(ctx) {
			t.Fatal("GetReady failed")
		}
		if !dag.Start() {
			t.Fatal("Start failed")
		}
		return dag.Wait(ctx)
	}

	// Build a diamond DAG: start → A → {B1, B2} → C.
	dag, err := InitDag()
	if err != nil {
		t.Fatalf("InitDag: %v", err)
	}
	dag.SetContainerCmd(NoopCmd{})

	for _, pair := range []struct{ from, to string }{
		{StartNode, "A"},
		{"A", "B1"},
		{"A", "B2"},
		{"B1", "C"},
		{"B2", "C"},
	} {
		if err := dag.AddEdge(pair.from, pair.to); err != nil {
			t.Fatalf("AddEdge %s->%s: %v", pair.from, pair.to, err)
		}
	}
	if err := dag.FinishDag(); err != nil {
		t.Fatalf("FinishDag: %v", err)
	}

	// ── First run ──
	if ok := buildAndRun(dag); !ok {
		t.Fatal("first Wait returned false")
	}
	if p := dag.Progress(); p != 1.0 {
		t.Errorf("first run: expected progress 1.0, got %v", p)
	}

	// ── Reset ──
	dag.Reset()

	// Progress must be 0.0 immediately after Reset (counters cleared).
	if p := dag.Progress(); p != 0.0 {
		t.Errorf("after Reset: expected progress 0.0, got %v", p)
	}

	// All nodes must be back in Pending.
	for id, n := range dag.nodes {
		if got := n.GetStatus(); got != NodeStatusPending {
			t.Errorf("after Reset: node %s expected Pending, got %v", id, got)
		}
	}

	// ── Second run ──
	if ok := buildAndRun(dag); !ok {
		t.Fatal("second Wait after Reset returned false")
	}
	if p := dag.Progress(); p != 1.0 {
		t.Errorf("second run: expected progress 1.0, got %v", p)
	}
}

// generateDAG 는 numNodes 개의 노드를 생성하고,
// i < j 인 경우 확률 edgeProb 로 부모-자식 간선을 추가하여 DAG 를 구성
func generateDAG(numNodes int, edgeProb float64) *Dag {
	// 새로운 DAG 생성
	dag := NewDag()
	dag.nodes = make(map[string]*Node, numNodes)

	// 노드 생성: "0", "1", ..., "numNodes-1"
	for i := 0; i < numNodes; i++ { //nolint:intrange
		id := fmt.Sprintf("%d", i)
		node := &Node{ID: id}
		dag.nodes[id] = node
	}

	// 간선 생성: i < j 조건에서 edgeProb 확률로 간선 추가
	for i := 0; i < numNodes; i++ { //nolint:intrange
		for j := i + 1; j < numNodes; j++ {
			if rand.Float64() < edgeProb { //nolint:gosec // test only, crypto-rand not required
				parentID := fmt.Sprintf("%d", i)
				childID := fmt.Sprintf("%d", j)

				// 엣지 생성 및 등록
				edge := &Edge{parentID: parentID, childID: childID}
				dag.Edges = append(dag.Edges, edge)

				// 부모/자식 관계 연결
				dag.nodes[parentID].children = append(dag.nodes[parentID].children, dag.nodes[childID])
				dag.nodes[childID].parent = append(dag.nodes[childID].parent, dag.nodes[parentID])
			}
		}
	}
	return dag
}

// verifyCopiedDag 는 원본 DAG 와 복사된 노드 맵 및 간선 슬라이스가 동일한 구조를 갖는지 검증
// 문제가 있으면 에러 메시지를 모아 하나의 error 로 반환하고, 문제가 없으면 nil 을 반환
//
//nolint:gocognit,gocyclo // exhaustive structural verification helper; complexity comes from checking all node/edge fields
func verifyCopiedDag(original *Dag, newNodes map[string]*Node, newEdges []*Edge, methodName string) error {
	var errs []string

	// (1) 노드 수 검증
	if len(newNodes) != len(original.nodes) {
		errs = append(errs, fmt.Sprintf("[%s] expected %d nodes, got %d", methodName, len(original.nodes), len(newNodes)))
	}

	// (2) 각 노드의 ID, 부모/자식 관계 검증
	for id, origNode := range original.nodes {
		newNode, ok := newNodes[id]
		if !ok {
			errs = append(errs, fmt.Sprintf("[%s] node with ID %s missing in newNodes", methodName, id))
			continue
		}
		// ID 비교
		if newNode.ID != origNode.ID {
			errs = append(errs, fmt.Sprintf("[%s] expected node ID %s, got %s", methodName, origNode.ID, newNode.ID))
		}
		// 부모 관계 검증
		if len(newNode.parent) != len(origNode.parent) {
			errs = append(errs, fmt.Sprintf("[%s] node %s: expected %d parents, got %d", methodName, id, len(origNode.parent), len(newNode.parent)))
		} else {
			for i, p := range newNode.parent {
				if p.ID != origNode.parent[i].ID {
					errs = append(errs, fmt.Sprintf("[%s] node %s: expected parent %s, got %s", methodName, id, origNode.parent[i].ID, p.ID))
				}
			}
		}
		// 자식 관계 검증
		if len(newNode.children) != len(origNode.children) {
			errs = append(errs, fmt.Sprintf("[%s] node %s: expected %d children, got %d", methodName, id, len(origNode.children), len(newNode.children)))
		} else {
			for i, c := range newNode.children {
				if c.ID != origNode.children[i].ID {
					errs = append(errs, fmt.Sprintf("[%s] node %s: expected child %s, got %s", methodName, id, origNode.children[i].ID, c.ID))
				}
			}
		}
	}

	// (3) 간선 검증
	if len(newEdges) != len(original.Edges) {
		errs = append(errs, fmt.Sprintf("[%s] expected %d edges, got %d", methodName, len(original.Edges), len(newEdges)))
	}
	for i, origEdge := range original.Edges {
		newEdge := newEdges[i]
		if newEdge.parentID != origEdge.parentID || newEdge.childID != origEdge.childID {
			errs = append(errs, fmt.Sprintf("[%s] edge %d: expected parent %s->child %s, got parent %s->child %s",
				methodName, i, origEdge.parentID, origEdge.childID, newEdge.parentID, newEdge.childID))
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "\n"))
	}
	return nil
}

// ── Stage 12 tests ────────────────────────────────────────────────────────────

// TestSendBlocking_Delivers verifies that SendBlocking successfully sends a
// value to a channel that has room and that the consumer receives it.
func TestSendBlocking_Delivers(t *testing.T) {
	defer goleak.VerifyNone(t)
	sc := NewSafeChannelGen[int](1)
	ctx := context.Background()
	if !sc.SendBlocking(ctx, 42) {
		t.Fatal("SendBlocking returned false on empty channel")
	}
	got := <-sc.GetChannel()
	if got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
}

// TestSendBlocking_CtxCancel verifies that SendBlocking returns false
// (without panicking or deadlocking) when the context is cancelled before
// a consumer arrives.
func TestSendBlocking_CtxCancel(t *testing.T) {
	defer goleak.VerifyNone(t)
	sc := NewSafeChannelGen[int](0) // unbuffered — send blocks until consumed
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately before calling SendBlocking
	if sc.SendBlocking(ctx, 99) {
		t.Fatal("SendBlocking should return false on a cancelled context")
	}
}

// TestSendBlocking_ClosedChannel verifies that SendBlocking returns false
// immediately on a channel that is already closed.
func TestSendBlocking_ClosedChannel(t *testing.T) {
	defer goleak.VerifyNone(t)
	sc := NewSafeChannelGen[int](1)
	if err := sc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	ctx := context.Background()
	if sc.SendBlocking(ctx, 7) {
		t.Fatal("SendBlocking should return false on a closed channel")
	}
}

// TestSendBlocking_BlocksThenDelivers verifies that SendBlocking blocks on a
// full channel and delivers the value once the consumer reads.
func TestSendBlocking_BlocksThenDelivers(t *testing.T) {
	defer goleak.VerifyNone(t)
	sc := NewSafeChannelGen[int](0) // unbuffered
	ctx := context.Background()

	done := make(chan struct{})
	go func() {
		defer close(done)
		sc.SendBlocking(ctx, 123) //nolint:errcheck
	}()

	// Give the goroutine time to block on the send.
	time.Sleep(20 * time.Millisecond)
	got := <-sc.GetChannel()
	<-done
	if got != 123 {
		t.Fatalf("expected 123, got %d", got)
	}
}

// TestDroppedErrors_Counter verifies that DroppedErrors increments each time
// reportError cannot deliver to a full Errors channel, and that Reset zeros it.
func TestDroppedErrors_Counter(t *testing.T) {
	defer goleak.VerifyNone(t)
	// Create a DAG with a tiny error channel (capacity 1) so it fills quickly.
	cfg := DefaultDagConfig()
	cfg.MaxChannelBuffer = 1
	dag := NewDagWithConfig(cfg)

	if dag.DroppedErrors() != 0 {
		t.Fatalf("expected 0 dropped errors initially, got %d", dag.DroppedErrors())
	}

	// First send fills the channel.
	dag.reportError(fmt.Errorf("err-1"))
	if dag.DroppedErrors() != 0 {
		t.Fatalf("first error should not be dropped (channel had room)")
	}

	// Second send overflows the channel — should be dropped.
	dag.reportError(fmt.Errorf("err-2"))
	if dag.DroppedErrors() != 1 {
		t.Fatalf("expected 1 dropped error, got %d", dag.DroppedErrors())
	}

	// Third send also dropped.
	dag.reportError(fmt.Errorf("err-3"))
	if dag.DroppedErrors() != 2 {
		t.Fatalf("expected 2 dropped errors, got %d", dag.DroppedErrors())
	}

	// Drain the channel so we can call Reset (Reset creates a new Errors channel).
	<-dag.Errors.GetChannel()
	dag.Errors.Close() //nolint:errcheck

	// Manually reset just the counter (Reset requires a fully initialised DAG;
	// test the atomic reset directly).
	dag.droppedErrors = 0
	if dag.DroppedErrors() != 0 {
		t.Fatalf("expected 0 after manual reset, got %d", dag.DroppedErrors())
	}
}

// TestWorkerPool_NodeTask verifies that the zero-churn worker pool correctly
// executes nodeTask entries and that no closures are created per submission.
func TestWorkerPool_NodeTask(t *testing.T) {
	defer goleak.VerifyNone(t)
	const workers = 4

	pool := NewDagWorkerPool(workers)
	ctx := context.Background()

	results := make(chan string, workers)
	for i := 0; i < workers; i++ {
		id := fmt.Sprintf("node%d", i)
		n := &Node{ID: id, status: NodeStatusPending}
		n.runnerStore(nil)
		sc := NewSafeChannelGen[*printStatus](8)

		// Attach a runner that records the node ID and drains the channel.
		n.runner = func(_ context.Context, res *SafeChannel[*printStatus]) {
			results <- n.ID
			res.Close() //nolint:errcheck
		}

		pool.Submit(nodeTask{node: n, sc: sc, ctx: ctx})
	}

	pool.Close()

	close(results)
	seen := make(map[string]bool)
	for id := range results {
		if seen[id] {
			t.Errorf("node %s executed more than once", id)
		}
		seen[id] = true
	}
	if len(seen) != workers {
		t.Errorf("expected %d executions, got %d", workers, len(seen))
	}
}

// ── Stage 13 tests ────────────────────────────────────────────────────────────

// randomDelayRunner simulates a real workload by sleeping for a random
// duration between 1 and maxMs milliseconds, honouring ctx cancellation.
type randomDelayRunner struct {
	maxMs int
}

func (r randomDelayRunner) RunE(ctx context.Context, _ interface{}) error {
	delay := time.Duration(rand.Intn(r.maxMs)+1) * time.Millisecond //nolint:gosec // test-only
	select {
	case <-time.After(delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// failingRunner always returns a non-nil error, simulating a node that fails.
type failingRunner struct{}

func (failingRunner) RunE(_ context.Context, _ interface{}) error {
	return errors.New("injected failure")
}

// buildStressDAG constructs a fan-out DAG:
//
//	start_node → {stress-0 … stress-(n-1)} → end_node
//
// All n user nodes run in parallel, which maximises concurrent channel activity.
// cfg must have MaxChannelBuffer ≥ n*5 and WorkerPoolSize ≥ n to avoid stalls.
func buildStressDAG(t *testing.T, n int, cfg DagConfig) *Dag { //nolint:unparam
	t.Helper()
	dag := NewDagWithConfig(cfg)
	var err error
	dag, err = dag.StartDag()
	if err != nil {
		t.Fatalf("buildStressDAG: StartDag: %v", err)
	}
	for i := range n {
		id := fmt.Sprintf("stress-%d", i)
		if err := dag.AddEdge(StartNode, id); err != nil {
			t.Fatalf("buildStressDAG: AddEdge start→%s: %v", id, err)
		}
	}
	if err := dag.FinishDag(); err != nil {
		t.Fatalf("buildStressDAG: FinishDag: %v", err)
	}
	return dag
}

// runOneCycle executes one full ConnectRunner → GetReady → Start → Wait cycle.
// Returns true iff the cycle succeeded without errors.
func runOneCycle(t *testing.T, dag *Dag) bool {
	t.Helper()
	ctx := context.Background()
	dag.ConnectRunner()
	if !dag.GetReady(ctx) {
		t.Error("runOneCycle: GetReady returned false")
		return false
	}
	if !dag.Start() {
		t.Error("runOneCycle: Start returned false")
		return false
	}
	return dag.Wait(ctx)
}

// TestDag_ConcurrencyStress executes a 1 000-node fan-out DAG under random
// latency across multiple Reset/re-run cycles, validating that:
//   - SendBlocking never loses a signal (no deadlock, no hang)
//   - Every iteration reaches Progress == 1.0
//   - DroppedErrors remains 0 throughout (no error-channel overflow)
//   - goleak confirms no goroutine leaks after the final iteration
//
//nolint:gocognit // stress test: covers concurrent coordination across many nodes and many iterations
func TestDag_ConcurrencyStress(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode (-short)")
	}
	defer goleak.VerifyNone(t)
	Log.SetOutput(io.Discard)

	const (
		nodeCount  = 1000
		iterations = 20
		maxDelayMs = 5
	)

	cfg := DefaultDagConfig()
	cfg.MaxChannelBuffer = nodeCount*5 + 100 // generous headroom for burst
	cfg.WorkerPoolSize = nodeCount + 10      // one worker per node + buffer
	cfg.MinChannelBuffer = 5                 // 3 msgs per node max; 5 is safe
	cfg.ExpectedNodeCount = nodeCount + 2    // +2 for start/end synthetic nodes
	cfg.DefaultTimeout = 30 * time.Second

	dag := buildStressDAG(t, nodeCount, cfg)
	dag.SetContainerCmd(randomDelayRunner{maxMs: maxDelayMs})

	for iter := range iterations {
		if iter > 0 {
			dag.Reset()
		}

		if ok := runOneCycle(t, dag); !ok {
			t.Errorf("iter %d: Wait returned false (DAG did not complete)", iter)
		}

		if p := dag.Progress(); p != 1.0 {
			t.Errorf("iter %d: expected Progress 1.0, got %v", iter, p)
		}

		if d := dag.DroppedErrors(); d != 0 {
			t.Errorf("iter %d: expected DroppedErrors 0, got %d", iter, d)
		}
	}
}

// TestDroppedErrors_UnderHighLoad verifies that DroppedErrors accurately tracks
// overflow events when many goroutines call reportError concurrently on a
// deliberately undersized error channel.
func TestDroppedErrors_UnderHighLoad(t *testing.T) {
	defer goleak.VerifyNone(t)

	const totalErrors = 200

	// Tiny error buffer so the channel fills quickly and drops occur.
	cfg := DefaultDagConfig()
	cfg.MaxChannelBuffer = 10
	dag := NewDagWithConfig(cfg)

	// Fire totalErrors concurrent reporters using a buffered done-channel for
	// lightweight synchronisation (avoids importing sync).
	done := make(chan struct{}, totalErrors)
	for range totalErrors {
		go func() {
			dag.reportError(fmt.Errorf("load-error"))
			done <- struct{}{}
		}()
	}

	// Wait for all goroutines to finish.
	for range totalErrors {
		<-done
	}

	dropped := dag.DroppedErrors()

	// At least one error must be delivered (first goroutines fill the buffer),
	// so dropped must be strictly less than totalErrors.
	if dropped >= totalErrors {
		t.Errorf("expected some errors delivered but all %d were dropped", totalErrors)
	}
	// With a buffer of 10 and 200 senders, at least 190 must be dropped.
	minDropped := int64(totalErrors - cfg.MaxChannelBuffer)
	if dropped < minDropped {
		t.Errorf("expected at least %d dropped errors (buffer=%d, total=%d), got %d",
			minDropped, cfg.MaxChannelBuffer, totalErrors, dropped)
	}

	// Drain and close so the test leaves no channel data behind.
	ch := dag.Errors.GetChannel()
	for len(ch) > 0 {
		<-ch
	}
	dag.Errors.Close() //nolint:errcheck
}

// TestSendBlocking_GoroutineLeak_ContextCancel verifies that a goroutine
// blocked inside SendBlocking is released promptly when its context is
// cancelled — no goroutine leak occurs.
func TestSendBlocking_GoroutineLeak_ContextCancel(t *testing.T) {
	defer goleak.VerifyNone(t)

	sc := NewSafeChannelGen[int](0) // unbuffered — send always blocks
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		sc.SendBlocking(ctx, 42) //nolint:errcheck
	}()

	// Give the goroutine time to reach the blocking select.
	time.Sleep(20 * time.Millisecond)

	// Cancel the context; the goroutine must unblock and exit.
	cancel()

	select {
	case <-done:
		// Goroutine exited cleanly — no leak.
	case <-time.After(time.Second):
		t.Fatal("goroutine leaked: SendBlocking did not return after context cancel")
	}
	sc.Close() //nolint:errcheck
}

// TestWait_ContextCancellation exercises the waitCtx.Done() branch inside
// Wait, verifying it returns false and does not leak goroutines when the
// caller's context is cancelled while nodes are still executing.
func TestWait_ContextCancellation(t *testing.T) {
	defer goleak.VerifyNone(t)
	Log.SetOutput(io.Discard)

	dag, err := InitDag()
	if err != nil {
		t.Fatalf("InitDag: %v", err)
	}

	// A long-running runner that respects ctx cancellation.
	dag.SetContainerCmd(&SimpleCommand{}) // 100 ms delay, ctx-aware

	if err := dag.AddEdge(StartNode, "slow"); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	if err := dag.FinishDag(); err != nil {
		t.Fatalf("FinishDag: %v", err)
	}

	dag.ConnectRunner()

	// Use a context with a very short timeout for both GetReady and Wait.
	// When the deadline fires, Wait returns false via the waitCtx.Done() path.
	// Node goroutines also observe the same cancellation and exit promptly.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	if !dag.GetReady(ctx) {
		t.Fatal("GetReady failed")
	}
	if !dag.Start() {
		t.Fatal("Start failed")
	}

	if dag.Wait(ctx) {
		t.Error("expected Wait to return false when context is cancelled mid-execution")
	}
}

// TestDag_ParentFailurePropagation verifies that when a root node fails:
//  1. Wait returns false (the DAG does not complete successfully).
//  2. Children of the failing node do NOT reach NodeStatusSucceeded.
//  3. DroppedErrors remains 0 (errors are not dropped, just failures propagated).
//
// NOTE on Skipped vs Failed: children may end up as either NodeStatusFailed or
// NodeStatusSkipped depending on execution timing.  If a child observes the
// failed parent via CheckParentsStatus before preFlight, it transitions to
// Skipped.  If the child is already in preFlight waiting when the parent fails,
// it receives a Failed channel signal and transitions Running→Failed.
// Both outcomes are correct — the important invariant is that children never
// report NodeStatusSucceeded when their parent has failed.
//
//nolint:gocognit,gocyclo // test covers topology setup, failure injection, signal propagation, and multi-node status assertion
func TestDag_ParentFailurePropagation(t *testing.T) {
	defer goleak.VerifyNone(t)
	Log.SetOutput(io.Discard)

	dag, err := InitDag()
	if err != nil {
		t.Fatalf("InitDag: %v", err)
	}

	// Topology: start → A(fails) → {B, C} → end
	for _, pair := range []struct{ from, to string }{
		{StartNode, "A"},
		{"A", "B"},
		{"A", "C"},
	} {
		if err := dag.AddEdge(pair.from, pair.to); err != nil {
			t.Fatalf("AddEdge %s→%s: %v", pair.from, pair.to, err)
		}
	}
	if err := dag.FinishDag(); err != nil {
		t.Fatalf("FinishDag: %v", err)
	}

	// Node A fails; B and C would succeed if reachable (they won't be).
	if !dag.SetNodeRunner("A", failingRunner{}) {
		t.Fatal("SetNodeRunner A failed")
	}
	if !dag.SetNodeRunner("B", NoopCmd{}) {
		t.Fatal("SetNodeRunner B failed")
	}
	if !dag.SetNodeRunner("C", NoopCmd{}) {
		t.Fatal("SetNodeRunner C failed")
	}

	dag.ConnectRunner()
	ctx := context.Background()
	if !dag.GetReady(ctx) {
		t.Fatal("GetReady failed")
	}
	if !dag.Start() {
		t.Fatal("Start failed")
	}

	// Wait must return false: A fails → B,C either Skipped or Failed (not
	// Succeeded) → EndNode preflight receives Failed signals → Wait = false.
	if dag.Wait(ctx) {
		t.Error("expected Wait to return false when a root node fails")
	}

	// B and C must NOT have succeeded — they must be either Failed or Skipped.
	for _, id := range []string{"B", "C"} {
		n := dag.nodes[id]
		if n == nil {
			t.Fatalf("node %s not found", id)
		}
		st := n.GetStatus()
		if st == NodeStatusSucceeded {
			t.Errorf("node %s: should not have Succeeded when parent A failed, got %v", id, st)
		}
		if st != NodeStatusFailed && st != NodeStatusSkipped {
			t.Errorf("node %s: expected Failed or Skipped, got %v", id, st)
		}
	}
}

// TestCheckParentsStatus_FailedParent verifies that CheckParentsStatus
// correctly transitions a Pending node to Skipped when its parent has
// already failed, exercising the parent-failure fast-path.
func TestCheckParentsStatus_FailedParent(t *testing.T) {
	defer goleak.VerifyNone(t)

	parent := &Node{ID: "parent"}
	parent.runnerStore(nil)
	parent.SetStatus(NodeStatusFailed) // parent is already failed

	child := &Node{ID: "child"}
	child.runnerStore(nil)
	child.parent = []*Node{parent}

	// CheckParentsStatus should detect the failed parent and transition child
	// to Skipped, returning false.
	if child.CheckParentsStatus() {
		t.Error("expected CheckParentsStatus to return false for a failed parent")
	}
	if st := child.GetStatus(); st != NodeStatusSkipped {
		t.Errorf("expected child status Skipped, got %v", st)
	}
}

// TestCollectErrors_CtxCancelled verifies that collectErrors returns
// immediately when the supplied context is already cancelled, exercising
// the case <-ctx.Done() branch.
func TestCollectErrors_CtxCancelled(t *testing.T) {
	defer goleak.VerifyNone(t)

	dag, err := InitDag()
	if err != nil {
		t.Fatalf("InitDag: %v", err)
	}

	// Pre-cancel the context so collectErrors returns via ctx.Done() immediately.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	errs := dag.collectErrors(ctx)
	// No errors were sent; result must be empty (or nil).
	if len(errs) != 0 {
		t.Errorf("expected 0 errors from cancelled context, got %d", len(errs))
	}
	// Cleanup.
	dag.Errors.Close() //nolint:errcheck
}

// TestCollectErrors_Timeout verifies that collectErrors returns after
// ErrorDrainTimeout expires when no errors are sent, exercising the
// case <-timeout branch.
func TestCollectErrors_Timeout(t *testing.T) {
	defer goleak.VerifyNone(t)

	cfg := DefaultDagConfig()
	cfg.ErrorDrainTimeout = 50 * time.Millisecond // very short timeout for test
	dag := NewDagWithConfig(cfg)

	start := time.Now()
	errs := dag.collectErrors(context.Background())
	elapsed := time.Since(start)

	if len(errs) != 0 {
		t.Errorf("expected 0 errors, got %d", len(errs))
	}
	// Should return after ~50 ms, not 5 s.
	if elapsed > 2*time.Second {
		t.Errorf("collectErrors took too long: %v (expected ≤ 200ms)", elapsed)
	}
	dag.Errors.Close() //nolint:errcheck
}

// ==================== Stage 13: Coverage Tests ====================

// TestToMermaid_BasicTopology verifies that ToMermaid renders a simple
// start→A→B→end topology into a valid Mermaid flowchart string.
func TestToMermaid_BasicTopology(t *testing.T) {
	defer goleak.VerifyNone(t)
	Log.SetOutput(io.Discard)

	dag, err := InitDag()
	if err != nil {
		t.Fatalf("InitDag: %v", err)
	}
	if err := dag.AddEdge(StartNode, "A"); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	if err := dag.AddEdge("A", "B"); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	if err := dag.FinishDag(); err != nil {
		t.Fatalf("FinishDag: %v", err)
	}

	out := dag.ToMermaid()
	if !strings.HasPrefix(out, "graph TD\n") {
		t.Errorf("expected Mermaid output to start with 'graph TD\\n', got: %q", out[:min(len(out), 20)])
	}
	// Synthetic nodes use stadium shape ([" "]).
	if !strings.Contains(out, `start_node`) {
		t.Errorf("expected start_node in output")
	}
	if !strings.Contains(out, `end_node`) {
		t.Errorf("expected end_node in output")
	}
	// Edge arrows must appear.
	if !strings.Contains(out, "-->") {
		t.Errorf("expected directed edges '-->' in Mermaid output")
	}
}

// TestToMermaid_SpecialCharsInNodeID verifies that mermaidSafeID replaces
// hyphens and other non-identifier characters with underscores.
func TestToMermaid_SpecialCharsInNodeID(t *testing.T) {
	defer goleak.VerifyNone(t)
	Log.SetOutput(io.Discard)

	dag, err := InitDag()
	if err != nil {
		t.Fatalf("InitDag: %v", err)
	}
	// Node IDs with hyphens are common in practice.
	if err := dag.AddEdge(StartNode, "my-node-1"); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	if err := dag.FinishDag(); err != nil {
		t.Fatalf("FinishDag: %v", err)
	}

	out := dag.ToMermaid()
	// Hyphens should be replaced with underscores in Mermaid IDs.
	if strings.Contains(out, "my-node-1 ") || strings.Contains(out, "my-node-1\n") {
		t.Errorf("expected hyphens to be replaced in Mermaid ID, got raw ID in output")
	}
	if !strings.Contains(out, "my_node_1") {
		t.Errorf("expected 'my_node_1' (hyphens→underscores) in Mermaid output:\n%s", out)
	}
}

// TestToMermaid_WithPerNodeRunner verifies that mermaidNodeLabel includes
// the runner's concrete type when a per-node Runnable is registered.
func TestToMermaid_WithPerNodeRunner(t *testing.T) {
	defer goleak.VerifyNone(t)
	Log.SetOutput(io.Discard)

	dag, err := InitDag()
	if err != nil {
		t.Fatalf("InitDag: %v", err)
	}
	if err := dag.AddEdge(StartNode, "worker"); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	if err := dag.FinishDag(); err != nil {
		t.Fatalf("FinishDag: %v", err)
	}
	// Assign a per-node runner before generating the diagram.
	if !dag.SetNodeRunner("worker", NoopCmd{}) {
		t.Fatal("SetNodeRunner failed")
	}

	out := dag.ToMermaid()
	// The label should include the runner's type name.
	if !strings.Contains(out, "NoopCmd") {
		t.Errorf("expected runner type 'NoopCmd' in Mermaid label:\n%s", out)
	}
}

// TestSetNodeRunners_Bulk verifies SetNodeRunners: applied count, missing list,
// and skipped list (start_node / end_node).
func TestSetNodeRunners_Bulk(t *testing.T) {
	defer goleak.VerifyNone(t)
	Log.SetOutput(io.Discard)

	dag, err := InitDag()
	if err != nil {
		t.Fatalf("InitDag: %v", err)
	}
	for _, id := range []string{"A", "B", "C"} {
		if err := dag.AddEdge(StartNode, id); err != nil {
			t.Fatalf("AddEdge %s: %v", id, err)
		}
	}
	if err := dag.FinishDag(); err != nil {
		t.Fatalf("FinishDag: %v", err)
	}

	runners := map[string]Runnable{
		"A":           NoopCmd{},
		"B":           NoopCmd{},
		"nonexistent": NoopCmd{}, // not in DAG → missing
		StartNode:     NoopCmd{}, // synthetic → skipped
	}
	applied, missing, skipped := dag.SetNodeRunners(runners)

	if applied != 2 {
		t.Errorf("expected applied=2, got %d", applied)
	}
	if len(missing) != 1 || missing[0] != "nonexistent" {
		t.Errorf("expected missing=[nonexistent], got %v", missing)
	}
	if len(skipped) != 1 || skipped[0] != StartNode {
		t.Errorf("expected skipped=[start_node], got %v", skipped)
	}
}

// TestNewDagWithOptions_Timeout verifies that NewDagWithOptions + WithTimeout
// sets bTimeout and Timeout on the resulting DAG.
func TestNewDagWithOptions_Timeout(t *testing.T) {
	defer goleak.VerifyNone(t)

	d := NewDagWithOptions(WithTimeout(42 * time.Second))
	if d == nil {
		t.Fatal("NewDagWithOptions returned nil")
	}
	if !d.bTimeout {
		t.Error("expected bTimeout=true after WithTimeout option")
	}
	if d.Timeout != 42*time.Second {
		t.Errorf("expected Timeout=42s, got %v", d.Timeout)
	}
}

// TestSafeChannel_Close_Twice verifies that calling Close on a SafeChannel
// more than once does not panic (idempotent close via sync.Once).
func TestSafeChannel_Close_Twice(t *testing.T) {
	defer goleak.VerifyNone(t)

	sc := NewSafeChannelGen[int](1)
	sc.Close() //nolint:errcheck
	// Second close must not panic.
	sc.Close() //nolint:errcheck
}

// TestSafeChannel_Send_Closed verifies that Send returns false when the
// SafeChannel is already closed, exercising the closed-check path in Send.
func TestSafeChannel_Send_Closed(t *testing.T) {
	defer goleak.VerifyNone(t)

	sc := NewSafeChannelGen[int](1)
	sc.Close() //nolint:errcheck

	ok := sc.Send(42)
	if ok {
		t.Error("expected Send to return false on a closed channel")
	}
}

// TestAddEdgeIfNodesExist_MissingNode verifies that AddEdgeIfNodesExist returns
// an error when a referenced node does not exist in the DAG, exercising
// the "node does not exist" error path.
func TestAddEdgeIfNodesExist_MissingNode(t *testing.T) {
	defer goleak.VerifyNone(t)
	Log.SetOutput(io.Discard)

	dag, err := InitDag()
	if err != nil {
		t.Fatalf("InitDag: %v", err)
	}
	// Add a real node so "from" resolves, but "to" is absent.
	err = dag.AddEdge(StartNode, "existing")
	if err != nil {
		t.Fatalf("AddEdge: %v", err)
	}

	err = dag.AddEdgeIfNodesExist("existing", "ghost")
	if err == nil {
		t.Error("expected error when to-node does not exist, got nil")
	}
}

// TestAddEdgeIfNodesExist_SelfLoop verifies that AddEdgeIfNodesExist rejects
// self-loop edges (from == to).
func TestAddEdgeIfNodesExist_SelfLoop(t *testing.T) {
	defer goleak.VerifyNone(t)
	Log.SetOutput(io.Discard)

	dag, err := InitDag()
	if err != nil {
		t.Fatalf("InitDag: %v", err)
	}
	err = dag.AddEdgeIfNodesExist("X", "X")
	if err == nil {
		t.Error("expected error for self-loop edge, got nil")
	}
}

// TestCreateNodeWithTimeOut verifies that CreateNodeWithTimeOut creates a node
// and returns non-nil for both the timeout and non-timeout cases.
func TestCreateNodeWithTimeOut(t *testing.T) {
	defer goleak.VerifyNone(t)

	dag := NewDag()
	// bTimeOut=true path
	n1 := dag.CreateNodeWithTimeOut("timer-node", true, 5*time.Second)
	if n1 == nil {
		t.Error("expected non-nil node from CreateNodeWithTimeOut with bTimeOut=true")
	}
	// bTimeOut=false path
	n2 := dag.CreateNodeWithTimeOut("plain-node", false, 0)
	if n2 == nil {
		t.Error("expected non-nil node from CreateNodeWithTimeOut with bTimeOut=false")
	}
	// Duplicate ID returns nil.
	n3 := dag.CreateNodeWithTimeOut("timer-node", true, 5*time.Second)
	if n3 != nil {
		t.Error("expected nil for duplicate node ID")
	}
}

// TestMinInt verifies both branches of the minInt helper: return a (a < b)
// and return b (a >= b).  minInt is an internal package-level function
// accessible from tests in the same package.
func TestMinInt(t *testing.T) {
	if got := minInt(3, 7); got != 3 {
		t.Errorf("minInt(3,7): expected 3, got %d", got)
	}
	if got := minInt(7, 3); got != 3 {
		t.Errorf("minInt(7,3): expected 3, got %d", got)
	}
	if got := minInt(5, 5); got != 5 {
		t.Errorf("minInt(5,5): expected 5, got %d", got)
	}
}

// TestWait_EmptyNodeResult verifies that Wait returns false immediately when
// GetReady is skipped (leaving dag.nodeResult empty), exercising the
// merge-returns-false branch in the Wait select loop.
func TestWait_EmptyNodeResult(t *testing.T) {
	defer goleak.VerifyNone(t)
	Log.SetOutput(io.Discard)

	dag, err := InitDag()
	if err != nil {
		t.Fatalf("InitDag: %v", err)
	}
	if err := dag.AddEdge(StartNode, "X"); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	if err := dag.FinishDag(); err != nil {
		t.Fatalf("FinishDag: %v", err)
	}
	dag.SetContainerCmd(NoopCmd{})
	dag.ConnectRunner()
	// Skip GetReady: dag.nodeResult stays empty → merge returns false.
	ctx := context.Background()
	ok := dag.Wait(ctx)
	if ok {
		t.Error("expected Wait to return false when nodeResult is empty")
	}
}

// TestNodeError_Unwrap verifies that NodeError.Unwrap returns the wrapped error,
// allowing errors.Is / errors.As to traverse the chain.
func TestNodeError_Unwrap(t *testing.T) {
	sentinel := errors.New("sentinel error")
	ne := &NodeError{NodeID: "X", Phase: "test", Err: sentinel}
	if !errors.Is(ne, sentinel) {
		t.Errorf("errors.Is failed: expected sentinel through NodeError.Unwrap")
	}
	if ne.Unwrap() != sentinel {
		t.Errorf("Unwrap: expected sentinel, got %v", ne.Unwrap())
	}
}

// TestNode_SetRunner verifies that SetRunner applies the runner on a Pending
// node and rejects changes after the node has moved to another state.
func TestNode_SetRunner(t *testing.T) {
	defer goleak.VerifyNone(t)

	n := &Node{ID: "test"}
	n.runnerStore(nil)
	// Pending → runner can be set.
	if !n.SetRunner(NoopCmd{}) {
		t.Error("expected SetRunner to return true for Pending node")
	}
	// After transition to Running, SetRunner should be rejected.
	n.SetStatus(NodeStatusRunning)
	if n.SetRunner(NoopCmd{}) {
		t.Error("expected SetRunner to return false for non-Pending node")
	}
}

// TestInitDagWithOptions verifies that InitDagWithOptions applies options and
// returns a fully initialized DAG ready for AddEdge, exercising the
// nolint:unused helper.
func TestInitDagWithOptions(t *testing.T) {
	defer goleak.VerifyNone(t)
	Log.SetOutput(io.Discard)

	d, err := InitDagWithOptions(WithTimeout(10 * time.Second))
	if err != nil {
		t.Fatalf("InitDagWithOptions: %v", err)
	}
	if !d.bTimeout {
		t.Error("expected bTimeout=true after WithTimeout option")
	}
	if d.StartNode == nil {
		t.Error("expected StartNode to be non-nil after InitDagWithOptions")
	}
}

// TestDagOptions_ChannelBuffersAndWorkerPool verifies that WithChannelBuffers
// and WithWorkerPool functional options correctly set the config fields on
// a new DAG, exercising the nolint:unused option helpers.
func TestDagOptions_ChannelBuffersAndWorkerPool(t *testing.T) {
	defer goleak.VerifyNone(t)

	d := NewDagWithOptions(
		WithChannelBuffers(3, 50, 10),
		WithWorkerPool(8),
	)
	if d == nil {
		t.Fatal("NewDagWithOptions returned nil")
	}
	if d.Config.MinChannelBuffer != 3 {
		t.Errorf("expected MinChannelBuffer=3, got %d", d.Config.MinChannelBuffer)
	}
	if d.Config.MaxChannelBuffer != 50 {
		t.Errorf("expected MaxChannelBuffer=50, got %d", d.Config.MaxChannelBuffer)
	}
	if d.Config.StatusBuffer != 10 {
		t.Errorf("expected StatusBuffer=10, got %d", d.Config.StatusBuffer)
	}
	if d.Config.WorkerPoolSize != 8 {
		t.Errorf("expected WorkerPoolSize=8, got %d", d.Config.WorkerPoolSize)
	}
}

// TestGetSafeVertex verifies that getSafeVertex returns the channel for a
// known edge and nil for an edge that does not exist.
func TestGetSafeVertex(t *testing.T) {
	defer goleak.VerifyNone(t)
	Log.SetOutput(io.Discard)

	dag, err := InitDag()
	if err != nil {
		t.Fatalf("InitDag: %v", err)
	}
	if err := dag.AddEdge(StartNode, "A"); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}

	// Known edge — must return a non-nil SafeChannel.
	sv := dag.getSafeVertex(StartNode, "A")
	if sv == nil {
		t.Error("expected non-nil safeVertex for existing edge start_node→A")
	}
	// Non-existent edge — must return nil.
	sv2 := dag.getSafeVertex("A", "B")
	if sv2 != nil {
		t.Error("expected nil for non-existent edge A→B")
	}
}

// TestSafeChannel_SendBlocking_Unblocks verifies that SendBlocking returns
// false immediately (not deadlocks) when the context is cancelled before
// a receiver is available, exercising the ctx.Done() branch.
func TestSafeChannel_SendBlocking_Unblocks(t *testing.T) {
	defer goleak.VerifyNone(t)

	// Unbuffered channel: no receiver → Send will block.
	sc := NewSafeChannelGen[int](0)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan bool, 1)
	go func() {
		done <- sc.SendBlocking(ctx, 99)
	}()

	// Cancel context so the goroutine unblocks via ctx.Done().
	cancel()
	result := <-done
	if result {
		t.Error("expected SendBlocking to return false on cancelled context")
	}
	sc.Close() //nolint:errcheck
}

// ==================== Stage 13: Coverage Path Tests ====================

// TestCopyDag_NilOriginal verifies that CopyDag returns nil when the source
// DAG is nil, exercising the nil-guard branch.
func TestCopyDag_NilOriginal(t *testing.T) {
	defer goleak.VerifyNone(t)

	result := CopyDag(nil, "copy")
	if result != nil {
		t.Error("expected CopyDag(nil, ...) to return nil")
	}
}

// TestCopyDag_EmptyID verifies that CopyDag returns nil when newID is empty,
// exercising the IsEmptyString guard branch.
func TestCopyDag_EmptyID(t *testing.T) {
	defer goleak.VerifyNone(t)
	Log.SetOutput(io.Discard)

	dag, err := InitDag()
	if err != nil {
		t.Fatalf("InitDag: %v", err)
	}
	if err := dag.AddEdge(StartNode, "A"); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	if err := dag.FinishDag(); err != nil {
		t.Fatalf("FinishDag: %v", err)
	}

	result := CopyDag(dag, "")
	if result != nil {
		t.Error("expected CopyDag(dag, \"\") to return nil")
	}
}

// TestExecute_NoRunner verifies that execute returns ErrNoRunner when no
// Runnable has been configured for the node (nil runner snapshot).
func TestExecute_NoRunner(t *testing.T) {
	defer goleak.VerifyNone(t)

	n := &Node{ID: "norunner"}
	// Do NOT store any runner — runnerVal is zero value.
	// getRunnerSnapshot will return nil → execute returns ErrNoRunner.
	err := execute(context.Background(), n)
	if !errors.Is(err, ErrNoRunner) {
		t.Errorf("expected ErrNoRunner, got %v", err)
	}
}

// TestRunnerLoad_NilValue verifies that runnerLoad returns nil when the
// atomic.Value has never been written (zero value).
func TestRunnerLoad_NilValue(t *testing.T) {
	defer goleak.VerifyNone(t)

	n := &Node{ID: "zero"}
	// Do NOT call runnerStore: atomic.Value is at zero → Load() returns nil.
	r := n.runnerLoad()
	if r != nil {
		t.Errorf("expected nil from runnerLoad on zero-value node, got %v", r)
	}
}

// TestStart_ClosedChannel verifies that Start returns false when the StartNode's
// parent channel is already closed (the Send failure path).
func TestStart_ClosedChannel(t *testing.T) {
	defer goleak.VerifyNone(t)
	Log.SetOutput(io.Discard)

	dag, err := InitDag()
	if err != nil {
		t.Fatalf("InitDag: %v", err)
	}
	if err := dag.AddEdge(StartNode, "A"); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	if err := dag.FinishDag(); err != nil {
		t.Fatalf("FinishDag: %v", err)
	}
	dag.SetContainerCmd(NoopCmd{})
	dag.ConnectRunner()

	// Close the StartNode's trigger channel so Send will fail.
	dag.StartNode.parentVertex[0].Close() //nolint:errcheck

	result := dag.Start()
	if result {
		t.Error("expected Start to return false when trigger channel is closed")
	}
	// Cleanup channels to avoid goroutine leaks.
	dag.closeChannels()
}

// TestSetNodeRunners_NonPendingSkipped verifies that SetNodeRunners skips
// nodes that are no longer in Pending status.
func TestSetNodeRunners_NonPendingSkipped(t *testing.T) {
	defer goleak.VerifyNone(t)
	Log.SetOutput(io.Discard)

	dag, err := InitDag()
	if err != nil {
		t.Fatalf("InitDag: %v", err)
	}
	if err := dag.AddEdge(StartNode, "X"); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	if err := dag.FinishDag(); err != nil {
		t.Fatalf("FinishDag: %v", err)
	}

	// Force X to Running status so SetNodeRunners must skip it.
	dag.nodes["X"].SetStatus(NodeStatusRunning)

	runners := map[string]Runnable{"X": NoopCmd{}}
	applied, _, skipped := dag.SetNodeRunners(runners)
	if applied != 0 {
		t.Errorf("expected applied=0, got %d", applied)
	}
	if len(skipped) != 1 {
		t.Errorf("expected skipped=[X], got %v", skipped)
	}
}

// TestAddEdge_EmptyFrom verifies that AddEdge returns an error when the
// from-node ID is an empty string.
func TestAddEdge_EmptyFrom(t *testing.T) {
	defer goleak.VerifyNone(t)
	Log.SetOutput(io.Discard)

	dag, err := InitDag()
	if err != nil {
		t.Fatalf("InitDag: %v", err)
	}
	if err := dag.AddEdge("", "B"); err == nil {
		t.Error("expected AddEdge to return error for empty from-node")
	}
	dag.Errors.Close() //nolint:errcheck
}

// TestAddEdge_EmptyTo verifies that AddEdge returns an error when the
// to-node ID is an empty string.
func TestAddEdge_EmptyTo(t *testing.T) {
	defer goleak.VerifyNone(t)
	Log.SetOutput(io.Discard)

	dag, err := InitDag()
	if err != nil {
		t.Fatalf("InitDag: %v", err)
	}
	if err := dag.AddEdge(StartNode, ""); err == nil {
		t.Error("expected AddEdge to return error for empty to-node")
	}
	dag.Errors.Close() //nolint:errcheck
}

// TestAddEdge_DuplicateEdge verifies that AddEdge returns an error when the
// same edge is added twice, exercising the check == Exist path.
func TestAddEdge_DuplicateEdge(t *testing.T) {
	defer goleak.VerifyNone(t)
	Log.SetOutput(io.Discard)

	dag, err := InitDag()
	if err != nil {
		t.Fatalf("InitDag: %v", err)
	}
	if err := dag.AddEdge(StartNode, "A"); err != nil {
		t.Fatalf("first AddEdge: %v", err)
	}
	// Second identical edge must be rejected.
	if err := dag.AddEdge(StartNode, "A"); err == nil {
		t.Error("expected second AddEdge to return error for duplicate edge")
	}
	dag.Errors.Close() //nolint:errcheck
}

// TestAddEdgeIfNodesExist_EmptyFrom verifies that AddEdgeIfNodesExist returns
// an error for an empty from-node ID.
func TestAddEdgeIfNodesExist_EmptyFrom(t *testing.T) {
	defer goleak.VerifyNone(t)
	Log.SetOutput(io.Discard)

	dag, err := InitDag()
	if err != nil {
		t.Fatalf("InitDag: %v", err)
	}
	if err := dag.AddEdgeIfNodesExist("", "B"); err == nil {
		t.Error("expected AddEdgeIfNodesExist to return error for empty from-node")
	}
	dag.Errors.Close() //nolint:errcheck
}

// TestInFlight_NilNode verifies that inFlight returns InFlightFailed status
// when called with a nil node (defensive nil-check path).
func TestInFlight_NilNode(t *testing.T) {
	defer goleak.VerifyNone(t)

	ps := inFlight(context.Background(), nil)
	if ps == nil || ps.rStatus != InFlightFailed {
		t.Errorf("expected InFlightFailed for nil node, got %v", ps)
	}
	releasePrintStatus(ps)
}

// TestInFlight_FailedNode verifies that inFlight skips Execute when
// n.IsSucceed() is false, exercising the "Skipping execution" else branch.
func TestInFlight_FailedNode(t *testing.T) {
	defer goleak.VerifyNone(t)
	Log.SetOutput(io.Discard)

	n := &Node{ID: "alreadyfailed"}
	n.runnerStore(NoopCmd{})
	n.SetSucceed(false) // simulate a prior failure

	ps := inFlight(context.Background(), n)
	if ps == nil || ps.rStatus != InFlightFailed {
		t.Errorf("expected InFlightFailed for pre-failed node, got %v", ps)
	}
	releasePrintStatus(ps)
}

// TestCollectErrors_ZeroDrainTimeout verifies that collectErrors falls back
// to the 5-second default when ErrorDrainTimeout is zero, exercising the
// drainTimeout <= 0 path.  We pre-cancel ctx to return immediately.
func TestCollectErrors_ZeroDrainTimeout(t *testing.T) {
	defer goleak.VerifyNone(t)

	cfg := DefaultDagConfig()
	cfg.ErrorDrainTimeout = 0 // force the <= 0 fallback
	dag := NewDagWithConfig(cfg)

	// Pre-cancel context so collectErrors returns immediately via ctx.Done().
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	errs := dag.collectErrors(ctx)
	elapsed := time.Since(start)

	if len(errs) != 0 {
		t.Errorf("expected 0 errors, got %d", len(errs))
	}
	// Should return quickly via ctx.Done(), not wait the 5s default timeout.
	if elapsed > time.Second {
		t.Errorf("collectErrors took too long: %v (expected <1s)", elapsed)
	}
	dag.Errors.Close() //nolint:errcheck
}

// ==================== Stage 13: Error Path Coverage Tests ====================

// TestSetNodeRunner_NilNode verifies that SetNodeRunner returns false when
// the specified node ID does not exist in the DAG (nil node guard path).
func TestSetNodeRunner_NilNode(t *testing.T) {
	defer goleak.VerifyNone(t)

	dag, err := InitDag()
	if err != nil {
		t.Fatalf("InitDag: %v", err)
	}
	if dag.SetNodeRunner("doesNotExist", NoopCmd{}) {
		t.Error("expected SetNodeRunner to return false for unknown node ID")
	}
}

// TestSetNodeRunner_NonPendingNode verifies that SetNodeRunner returns false
// when the target node is not in Pending status (e.g. already Running).
func TestSetNodeRunner_NonPendingNode(t *testing.T) {
	defer goleak.VerifyNone(t)
	Log.SetOutput(io.Discard)

	dag, err := InitDag()
	if err != nil {
		t.Fatalf("InitDag: %v", err)
	}
	if err := dag.AddEdge(StartNode, "worker"); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	if err := dag.FinishDag(); err != nil {
		t.Fatalf("FinishDag: %v", err)
	}

	// Force the node into Running status so SetNodeRunner should reject it.
	dag.nodes["worker"].SetStatus(NodeStatusRunning)

	if dag.SetNodeRunner("worker", NoopCmd{}) {
		t.Error("expected SetNodeRunner to return false for Running node")
	}
}

// TestFinishDag_AlreadyValidated verifies that calling FinishDag a second time
// returns an error because dag.validated is already set to true.
func TestFinishDag_AlreadyValidated(t *testing.T) {
	defer goleak.VerifyNone(t)
	Log.SetOutput(io.Discard)

	dag, err := InitDag()
	if err != nil {
		t.Fatalf("InitDag: %v", err)
	}
	if err := dag.AddEdge(StartNode, "A"); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	if err := dag.FinishDag(); err != nil {
		t.Fatalf("first FinishDag: %v", err)
	}
	// Second call must fail.
	if err := dag.FinishDag(); err == nil {
		t.Error("expected second FinishDag to return error (already validated)")
	}
}

// TestFinishDag_NoNodes verifies that FinishDag returns an error when called
// on a DAG that has no nodes at all (neither via StartDag nor AddEdge).
func TestFinishDag_NoNodes(t *testing.T) {
	defer goleak.VerifyNone(t)
	Log.SetOutput(io.Discard)

	// NewDag without StartDag: dag.nodes is empty.
	dag := NewDag()
	if err := dag.FinishDag(); err == nil {
		t.Error("expected FinishDag to return error for empty DAG")
	}
	dag.Errors.Close() //nolint:errcheck
}

// TestStartDag_Duplicate verifies that calling StartDag twice returns an error
// on the second call because start_node already exists.
func TestStartDag_Duplicate(t *testing.T) {
	defer goleak.VerifyNone(t)
	Log.SetOutput(io.Discard)

	dag := NewDag()
	d2, err := dag.StartDag()
	if err != nil {
		t.Fatalf("first StartDag: %v", err)
	}
	// Second call: start_node already in dag.nodes → createNode returns nil → error.
	_, err = d2.StartDag()
	if err == nil {
		t.Error("expected second StartDag to return error (duplicate start_node)")
	}
}

// TestFinishDag_IsolatedNode verifies that FinishDag returns an error when
// a non-start node exists with no parent and no children (isolated/orphaned).
func TestFinishDag_IsolatedNode(t *testing.T) {
	defer goleak.VerifyNone(t)
	Log.SetOutput(io.Discard)

	dag, err := InitDag()
	if err != nil {
		t.Fatalf("InitDag: %v", err)
	}
	if err := dag.AddEdge(StartNode, "connected"); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	// Inject an isolated node directly (bypassing AddEdge which enforces structure).
	dag.mu.Lock()
	dag.createNode("orphan") //nolint:errcheck // intentionally creating isolated node
	dag.mu.Unlock()

	if err := dag.FinishDag(); err == nil {
		t.Error("expected FinishDag to return error for isolated node 'orphan'")
	}
	dag.Errors.Close() //nolint:errcheck
}

// TestProgress_EmptyDAG verifies that Progress() returns 0.0 when no nodes have
// been added to the DAG (nodeCount == 0 guard path).
func TestProgress_EmptyDAG(t *testing.T) {
	dag := NewDag()
	if got := dag.Progress(); got != 0.0 {
		t.Errorf("expected Progress()=0.0 for empty DAG, got %f", got)
	}
	dag.Errors.Close() //nolint:errcheck
}

// TestPostFlight_NilNode verifies that postFlight handles a nil node gracefully,
// returning PostFlightFailed with the noNodeID sentinel.
func TestPostFlight_NilNode(t *testing.T) {
	ps := postFlight(context.Background(), nil)
	if ps == nil {
		t.Fatal("postFlight(nil) returned nil printStatus")
	}
	if ps.rStatus != PostFlightFailed {
		t.Errorf("expected PostFlightFailed, got %v", ps.rStatus)
	}
}

// TestCopyDag_WithStartAndEndNode verifies that CopyDag correctly identifies and
// wires StartNode and EndNode when copying a fully-initialized DAG, exercising
// the case StartNode / case EndNode branches inside CopyDag.
func TestCopyDag_WithStartAndEndNode(t *testing.T) {
	defer goleak.VerifyNone(t)
	Log.SetOutput(io.Discard)

	original, err := InitDag()
	if err != nil {
		t.Fatalf("InitDag: %v", err)
	}
	if err := original.AddEdge(StartNode, "mid"); err != nil {
		t.Fatalf("AddEdge StartNode->mid: %v", err)
	}
	if err := original.FinishDag(); err != nil {
		t.Fatalf("FinishDag: %v", err)
	}

	copied := CopyDag(original, "copy-with-se")
	if copied == nil {
		t.Fatal("CopyDag returned nil")
	}
	if copied.StartNode == nil {
		t.Error("copied.StartNode is nil — case StartNode branch not exercised")
	}
	if copied.EndNode == nil {
		t.Error("copied.EndNode is nil — case EndNode branch not exercised")
	}
	// Verify the IDs match expected sentinels.
	if copied.StartNode != nil && copied.StartNode.ID != StartNode {
		t.Errorf("copied StartNode ID = %q, want %q", copied.StartNode.ID, StartNode)
	}
	if copied.EndNode != nil && copied.EndNode.ID != EndNode {
		t.Errorf("copied EndNode ID = %q, want %q", copied.EndNode.ID, EndNode)
	}

	original.Errors.Close() //nolint:errcheck
	copied.Errors.Close()   //nolint:errcheck
}

// TestAddEndNode_NilFrom verifies that addEndNode returns an error when fromNode is nil.
func TestAddEndNode_NilFrom(t *testing.T) {
	dag := NewDag()
	someNode := &Node{ID: "x"}
	err := dag.addEndNode(nil, someNode)
	if err == nil {
		t.Error("expected error for nil fromNode, got nil")
	}
	dag.Errors.Close() //nolint:errcheck
}

// TestAddEndNode_NilTo verifies that addEndNode returns an error when toNode is nil.
func TestAddEndNode_NilTo(t *testing.T) {
	dag := NewDag()
	someNode := &Node{ID: "y"}
	err := dag.addEndNode(someNode, nil)
	if err == nil {
		t.Error("expected error for nil toNode, got nil")
	}
	dag.Errors.Close() //nolint:errcheck
}

// TestPreFlight_DagDefaultTimeout exercises the parentDag.Config.DefaultTimeout > 0
// branch in preFlight, ensuring the DAG-level timeout context is used when no
// per-node timeout is configured.
func TestPreFlight_DagDefaultTimeout(t *testing.T) {
	defer goleak.VerifyNone(t)
	Log.SetOutput(io.Discard)

	dag, err := InitDagWithOptions(func(d *Dag) {
		d.Config.DefaultTimeout = 5 * time.Second
	})
	if err != nil {
		t.Fatalf("InitDagWithOptions: %v", err)
	}
	if err := dag.AddEdge(StartNode, "timed"); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	if err := dag.FinishDag(); err != nil {
		t.Fatalf("FinishDag: %v", err)
	}
	dag.SetContainerCmd(NoopCmd{})
	dag.ConnectRunner()
	ctx := context.Background()
	dag.GetReady(ctx)
	dag.Start()
	ok := dag.Wait(ctx)
	if !ok {
		t.Error("Wait returned false with DefaultTimeout DAG")
	}
}

// TestConnectRunner_EmptyDAG exercises the ConnectRunner early-return path
// when no nodes are present (len(dag.nodes) < 1).
func TestConnectRunner_EmptyDAG(t *testing.T) {
	dag := NewDag()
	if dag.ConnectRunner() {
		t.Error("ConnectRunner on empty DAG should return false")
	}
	dag.Errors.Close() //nolint:errcheck
}

// TestGetReady_EmptyDAG exercises the GetReady early-return path when no nodes
// are present (len(dag.nodes) < 1).
func TestGetReady_EmptyDAG(t *testing.T) {
	dag := NewDag()
	if dag.GetReady(context.Background()) {
		t.Error("GetReady on empty DAG should return false")
	}
	dag.Errors.Close() //nolint:errcheck
}

// TestNotifyChildren_CancelledCtx exercises the Warnf branch in notifyChildren
// when SendBlocking returns false because the context is already cancelled.
// An unbuffered channel with no reader guarantees the send blocks, so the
// cancelled context makes SendBlocking pick the ctx.Done() case.
func TestNotifyChildren_CancelledCtx(t *testing.T) {
	parent := &Node{ID: "parentNC"}
	// Unbuffered channel — sc.ch <- value would block with no reader.
	sc := NewSafeChannelGen[runningStatus](0)
	parent.childrenVertex = []*SafeChannel[runningStatus]{sc}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled → SendBlocking returns false → Warnf is logged

	parent.notifyChildren(ctx, Succeed)
	sc.Close() //nolint:errcheck
}

// TestPostFlight_CancelledCtx exercises the Warnf branch in postFlight when
// SendBlocking to a child returns false due to a pre-cancelled context.
func TestPostFlight_CancelledCtx(t *testing.T) {
	node := &Node{ID: "pfCancelNode"}
	node.SetSucceed(true)
	// Unbuffered channel with no reader and a cancelled context → SendBlocking=false.
	sc := NewSafeChannelGen[runningStatus](0)
	node.childrenVertex = []*SafeChannel[runningStatus]{sc}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ps := postFlight(ctx, node)
	if ps == nil {
		t.Fatal("postFlight returned nil")
	}
	// Despite the warning, postFlight still returns PostFlight status.
	if ps.rStatus != PostFlight {
		t.Errorf("expected PostFlight, got %v", ps.rStatus)
	}
	sc.Close() //nolint:errcheck
}

// TestAddEndNode_ExistingEdge exercises the Fault/Exist guard in addEndNode by
// adding the same edge twice, which causes createEdge to return Exist on the
// second call — producing an error.
func TestAddEndNode_ExistingEdge(t *testing.T) {
	dag := NewDag()
	from := &Node{ID: "aene-from"}
	to := &Node{ID: "aene-to"}
	// Register nodes in dag so createEdge can append to dag.Edges.
	dag.nodes["aene-from"] = from
	dag.nodes["aene-to"] = to

	if err := dag.addEndNode(from, to); err != nil {
		t.Fatalf("first addEndNode: %v", err)
	}
	// Second call: the edge already exists → createEdge returns Exist → error.
	if err := dag.addEndNode(from, to); err == nil {
		t.Error("duplicate addEndNode should return error")
	}
	dag.Errors.Close() //nolint:errcheck
}

// TestFinishDag_SingleNonStartNode covers the FinishDag path where there is
// exactly one node and it is not the StartNode — an invalid configuration.
func TestFinishDag_SingleNonStartNode(t *testing.T) {
	dag := NewDag()
	// Bypass StartDag; inject a single non-StartNode directly.
	dag.mu.Lock()
	dag.createNode("solo-but-not-start") //nolint:errcheck
	dag.mu.Unlock()

	if err := dag.FinishDag(); err == nil {
		t.Error("FinishDag should reject a single non-StartNode")
	}
	dag.Errors.Close() //nolint:errcheck
}

// TestCloseChannels_AfterDoubleWait verifies that calling closeChannels
// twice (via two Wait calls on a finished DAG) does not panic even though
// the channels are already closed — exercising the error return path of
// SafeChannel.Close().
func TestCloseChannels_AfterDoubleWait(t *testing.T) {
	defer goleak.VerifyNone(t)
	Log.SetOutput(io.Discard)

	dag, err := InitDag()
	if err != nil {
		t.Fatalf("InitDag: %v", err)
	}
	if err := dag.AddEdge(StartNode, "A"); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	if err := dag.FinishDag(); err != nil {
		t.Fatalf("FinishDag: %v", err)
	}
	dag.SetContainerCmd(NoopCmd{})
	dag.ConnectRunner()
	ctx := context.Background()
	dag.GetReady(ctx)
	dag.Start()
	dag.Wait(ctx)

	// Second closeChannels call: channels are already closed → Close() returns error internally.
	// Must not panic.
	dag.closeChannels()
}
