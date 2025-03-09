```aiignore
package dag_go

import (
	"context"
	"github.com/stretchr/testify/assert"
	"testing"
)

// https://github.com/stretchr/testify
// https://pkg.go.dev/github.com/google/uuid#IsInvalidLengthError
// https://go101.org/article/channel-closing.html

func TestSimpleDag(t *testing.T) {
	assert := assert.New(t)

	//runnable := Connect()
	dag := NewDag()
	//dag.SetContainerCmd(runnable)

	// create dag
	dag.AddEdge(dag.StartNode.Id, "1")
	dag.AddEdge("1", "2")
	dag.AddEdge("1", "3")
	dag.AddEdge("1", "4")
	dag.AddEdge("2", "5")
	dag.AddEdge("5", "6")

	err := dag.FinishDag()
	if err != nil {
		t.Errorf("%+v", err)
	}
	ctx := context.Background()

	dag.ConnectRunner()
	dag.GetReady(ctx)
	b1 := dag.Start()
	assert.Equal(true, b1, "true")

	b2 := dag.Wait(ctx)
	assert.Equal(true, b2, "true")

}

func TestDetectCycle(t *testing.T) {
	assert := assert.New(t)

	// Create a new DAG
	dag := NewDag()

	// Add edges to the DAG
	dag.AddEdge(dag.StartNode.Id, "1")
	dag.AddEdge("1", "2")
	dag.AddEdge("1", "3")
	dag.AddEdge("1", "4")
	dag.AddEdge("2", "5")
	dag.AddEdge("5", "6")

	err := dag.FinishDag()
	if err != nil {
		t.Errorf("%+v", err)
	}

	// Define the visit map
	visit := make(map[string]bool)
	for id := range dag.nodes {
		visit[id] = false
	}

	// Test the detectCycle function
	cycle, end := dag.detectCycle(dag.StartNode.Id, dag.StartNode.Id, visit)

	// Assert that there is no cycle and the entire graph has been traversed
	assert.Equal(false, cycle, "Expected no cycle")
	assert.Equal(true, end, "Expected the entire graph to be traversed")
}

```

```aiignore
func preFlight(ctx context.Context, n *Node) *printStatus {
	if n == nil {
		panic(fmt.Errorf("node is nil"))
		// (do not erase) 안정화 버전 나올때는 panic 을 리턴으로 처리
		//return &printStatus{PreflightFailed, noNodeId}
	}
	// (do not erase) goroutine 디버깅용
	/*debugger.SetLabels(func() []string {
		return []string{
			"preFlight: nodeId", n.Id,
		}
	})*/

	// 성공하면 context 사용한다.
	eg, _ := errgroup.WithContext(ctx)
	i := len(n.parentVertex) // 부모 채널의 수
	for j := 0; j < i; j++ {
		// (do not erase) 중요!! 여기서 들어갈 변수를 세팅않해주면 에러남.
		k := j
		c := n.parentVertex[k]
		eg.Go(func() error {
			result := <-c
			if result == Failed {
				fmt.Println("failed", n.Id)
				return fmt.Errorf("failed")
			}
			return nil
		})
	}
	if err := eg.Wait(); err == nil { // 대기
		n.succeed = true
		fmt.Println("Preflight", n.Id)
		return &printStatus{Preflight, n.Id}
	}
	n.succeed = false
	fmt.Println("PreflightFailed", n.Id)
	return &printStatus{PreflightFailed, noNodeId}
}

func preFlightT(ctx context.Context, n *Node) *printStatus {
	if n == nil {
		panic(fmt.Errorf("node is nil"))
		// (do not erase) 안정화 버전 나올때는 panic 을 리턴으로 처리
		//return &printStatus{PreflightFailed, noNodeId}
	}
	// 성공하면 context 사용한다.
	eg, _ := errgroup.WithContext(ctx)
	i := len(n.parentVertex) // 부모 채널의 수
	var try bool
	for j := 0; j < i; j++ {
		// (do not erase) 중요!! 여기서 들어갈 변수를 세팅않해주면 에러남.
		k := j
		c := n.parentVertex[k]
		try = eg.TryGo(func() error {
			result := <-c
			if result == Failed {
				return fmt.Errorf("failed")
			}
			return nil
		})
		if !try {
			break
		}
	}
	if try {
		if err := eg.Wait(); err == nil { // 대기
			n.succeed = true
			return &printStatus{Preflight, n.Id}
		}
	}

	n.succeed = false
	return &printStatus{PreflightFailed, noNodeId}
}

// CopyDag dag 를 복사함.
func CopyDag(original *Dag, Id string) (copied *Dag) {
	if original == nil {
		return nil
	}
	copied = &Dag{}

	if utils.IsEmptyString(original.Pid) == false {
		copied.Pid = original.Pid
	}
	if utils.IsEmptyString(Id) {
		return nil
	}
	copied.Id = Id
	ns, edges := copyDag(original)
	copied.nodes = ns
	copied.Edges = edges

	for _, n := range ns {
		n.parentDag = copied
		if n.Id == StartNode {
			copied.StartNode = n
		}
		if n.Id == EndNode {
			copied.EndNode = n
		}
	}
	copied.validated = original.validated
	copied.RunningStatus = make(chan *printStatus, Max)
	//TODO 생략 추후 넣어줌
	// 에러를 모으는 용도.
	//errLogs []*systemError
	copied.Timeout = original.Timeout
	copied.bTimeout = original.bTimeout
	copied.ContainerCmd = original.ContainerCmd
	return
}

// cycle 이면 true, cycle 이 아니면 false
// 이상태서 리턴 중복되는 것과 recursive 확인하자.
// 첫번째 bool 은 circle 인지, 두번째 bool 은 다 돌았는지
// BUG(seoy): 이상 현상 발생
// detectCycle runningStatus 과 start, restart, pause, stop 등 추가 후 버그 개선
func (dag *Dag) detectCycle(startNodeId string, endNodeId string, visit map[string]bool) (bool, bool) {
	// 복사 할때 원본이 복사가 되는지 확인해서
	// getLefMostNode 할때 지워지는지 확인해야한다.
	var (
		nextNode    *Node // 자식 노 추후 이름 바꿈.
		currentNode *Node // current node 추후 이름 바꿈.
		curId       string
		cycle       bool // 서클인지 확인, circle 이면 true
		end         bool // 다 순회했는지 확인, 다 순회하면 true
	)
	// TODO dag.nodes nil check 해야함.
	ns, check := cloneGraph(dag.nodes)
	// cycle 이면 true, cycle 이 아니면 false
	if check {
		return true, false
	}

	curId = endNodeId

	cycle = false
	end = false

	// visit 이 모두 true 인데 이동할 자식이 있는 경우는 circle 이다.
	if checkVisit(visit) {
		// 즉, getLefMostNode 에서는 지워지는데 ns 를 지운다. 그리고 모두 방문했는데 자식노드가 있다면 그것은 cycle 이다.
		n := getNode(endNodeId, dag.nodes)
		if len(n.children) > 0 {
			return true, end // circle, end
		}
	}

	// 그외의 조건들일 경우 방문처리를 진행함.
	// 여기서 오해하지 말아야 할 경우는 detectCycle 는 recursive func 임.
	// 처음 초기 설정 값은 start_node_id 와 end_node_id 가 start_node_id 로 설정될 것임.
	visit[endNodeId] = true

	// DFS(깊이우선 방식으로 graph 를 순회함.
	currentNode = getNode(endNodeId, ns)

	if currentNode != nil {
		nextNode = getNextNode(currentNode)

		if nextNode != nil {
			endNodeId = nextNode.Id
		}
	}

	for nextNode != nil {
		cycle, end = dag.detectCycle(startNodeId, endNodeId, visit) // 파라미터의 temp1 는 부모를 넣저.
		fmt.Println(ns[curId].Id)                                   // TODO 이거 주석 처리했다고 리턴값이 달라짐.

		end = checkVisit(visit)

		if end {
			return cycle, end
		}

		// cycle 이면 리턴
		if cycle {
			return cycle, end
		}

		// 여기서 부모 노드이다.
		parentNode := getNode(curId, ns)
		if parentNode != nil {
			nextNode = getNextNode(parentNode)
			if nextNode != nil {
				endNodeId = nextNode.Id
				end = false
			}
			end = true
			nextNode = nil
		}
	}
	return cycle, end
}

// gpt 수정 버전본.
func (dag *Dag) detectCycleT(startNodeId string, endNodeId string, visit map[string]bool) (bool, bool) {
	var (
		nextNode    *Node // The next node to visit
		currentNode *Node // The current node being visited
		curId       string
		cycle       bool // True if a cycle is detected
		end         bool // True if the entire graph has been traversed
	)

	// Clone the graph and check for a cycle.
	ns, check := cloneGraph(dag.nodes)
	if check {
		return true, false // A cycle was detected during cloning
	}

	curId = endNodeId
	cycle = false
	end = false

	// If all nodes are visited, but there are still children left to visit, then it is a cycle.
	if checkVisit(visit) {
		n := getNode(endNodeId, dag.nodes)
		if len(n.children) > 0 {
			return true, end // It's a cycle
		}
	}

	// Mark the current node as visited.
	visit[endNodeId] = true

	// Perform a Depth First Search (DFS).
	currentNode = getNode(endNodeId, ns)
	if currentNode != nil {
		nextNode = getNextNode(currentNode)
		if nextNode != nil {
			endNodeId = nextNode.Id
		}
	}

	for nextNode != nil {
		// Recursive call to detect cycles.
		cycle, end = dag.detectCycle(startNodeId, endNodeId, visit)
		if end || cycle {
			return cycle, end // If a cycle is detected or all nodes are visited, exit the loop.
		}

		// Move to the next node.
		parentNode := getNode(curId, ns)
		if parentNode != nil {
			nextNode = getNextNode(parentNode)
			if nextNode != nil {
				endNodeId = nextNode.Id
				end = false
			}
			end = true
			nextNode = nil
		}
	}

	return cycle, end
}
```