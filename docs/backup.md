### 절대 지우지 말것. 책이나 정리할때 필요.

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


package dag_go

import (
	"context"
	"fmt"
	"golang.org/x/sync/errgroup"
)

// 나중에 책쓸대 사용하기 위한 histroy
/*
* 새롭게 교체 되면 _old_날짜 입력 -> 일단 테스트 완료.
* 테스트 전인 메서드는 원본 메서드에 T 를 붙임.
 */

// Deprecated : 테스트 완료.
// preFlight_old_250306
func preFlight_old_250306(ctx context.Context, n *Node) *printStatus {
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
		Log.Println("Preflight", n.Id)
		return &printStatus{Preflight, n.Id}
	}
	n.succeed = false
	Log.Println("PreflightFailed", n.Id)
	return &printStatus{PreflightFailed, noNodeId}
}

// Deprecated : 테스트 완료.
// preFlight_old_250305
func preFlight_old_250305(ctx context.Context, n *Node) *printStatus {
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

func copyDag_old_250307(original *Dag) (map[string]*Node, []*Edge) {
	num := len(original.nodes)
	if num < 1 {
		return nil, nil
	}
	var nodes map[string]*Node
	nodes = make(map[string]*Node, len(original.nodes))

	// 1. 먼저 모든 노드들을 복사한다. (노드자체 가존재하지 않을 수 있으므로 부모 자식 아직 추가 않해줌.)
	// TODO 아래 shallow copy 가 이루어지는 부분이 있는데, 이것까지 copy 해야할 지 생각해야함.
	for _, n := range original.nodes {
		node := new(Node)
		node.Id = n.Id
		node.ImageName = n.ImageName
		node.RunCommand = n.RunCommand
		node.Commands = n.Commands
		node.succeed = n.succeed

		nodes[node.Id] = node
	}
	// 2. 부모 자식 노드들을 추가해줌.
	for _, n := range original.nodes {
		p := len(n.parent)
		ch := len(n.children)
		// 부모노드들을 추가한다.
		for i := 0; i < p; i++ {
			nodes[n.Id].parent = append(nodes[n.Id].parent, nodes[n.parent[i].Id])
		}
		// 자식 노드들을 추가한다.
		for i := 0; i < ch; i++ {
			nodes[n.Id].children = append(nodes[n.Id].children, nodes[n.children[i].Id])
		}
	}
	// 3. Vertex(channel) 을 복사한다.
	edges := CopyEdge(original.Edges)
	num = len(edges)
	if num > 0 {
		for _, node := range nodes {
			es := findEdgeFromParentId(edges, node.Id)
			for _, e := range es {
				node.childrenVertex = append(node.childrenVertex, e.vertex)
			}
			es = findEdgeFromChildId(edges, node.Id)
			for _, e := range es {
				node.parentVertex = append(node.parentVertex, e.vertex)
			}
		}
	}

	// 4. xml 을 위해 from, to 복사 해준다. (추후 없어질 수 있음.)
	for _, n := range original.nodes {
		for _, id := range n.from {
			nodes[n.Id].from = append(nodes[n.Id].from, id)
		}

		for _, id := range n.to {
			nodes[n.Id].to = append(nodes[n.Id].to, id)
		}
	}
	return nodes, edges
}

func copyDagT(original *Dag) (map[string]*Node, []*Edge) {
	// 원본에 노드가 없으면 nil 반환
	if len(original.nodes) == 0 {
		return nil, nil
	}

	// 1. 노드의 기본 정보(ID)만 복사한 새 맵 생성
	newNodes := make(map[string]*Node, len(original.nodes))
	for _, n := range original.nodes {
		// 필요한 최소한의 정보만 복사
		newNode := &Node{
			Id: n.Id,
			// 기타 필드는 cycle 검증에 필요하지 않으므로 생략
		}
		newNodes[newNode.Id] = newNode
	}

	// 2. 원본 노드의 부모/자식 관계를 이용하여 새 노드들의 포인터 연결
	for _, n := range original.nodes {
		newNode := newNodes[n.Id]
		// 부모 노드 연결
		for _, parent := range n.parent {
			if copiedParent, ok := newNodes[parent.Id]; ok {
				newNode.parent = append(newNode.parent, copiedParent)
			}
		}
		// 자식 노드 연결
		for _, child := range n.children {
			if copiedChild, ok := newNodes[child.Id]; ok {
				newNode.children = append(newNode.children, copiedChild)
			}
		}
	}

	// 3. 간선(Edge) 복사: detectCycle 에 필요하다면 parentId와 childId만 복사
	newEdges := make([]*Edge, len(original.Edges))
	for i, e := range original.Edges {
		newEdges[i] = &Edge{
			parentId: e.parentId,
			childId:  e.childId,
			// vertex 등 기타 정보는 cycle 검증에 필요하지 않으므로 생략
		}
	}

	return newNodes, newEdges
}

// deprecated : 에정
// Debug 목적으로 스택? 두개 만들어서 채널에서 보내는 값과, 받는  값각각 넣어서 비교해본다.
// (do not erase) close 해주는 것 : func (dag *Dag) Wait(ctx context.Context) bool  에서 defer close(dag.RunningStatus) 해줌
// (do not erase) 너무 중요.@@@@ 채널 close 방식 확인하자. https://go101.org/article/channel-closing.html 너무 좋은 자료. 왜 제목을 101 이라고 했지 중급이상인데.
// setFunc commit by seoy
// https://stackoverflow.com/questions/15715605/multiple-goroutines-listening-on-one-channel
// https://go.dev/ref/mem#tmp_7 읽자.
// https://umi0410.github.io/blog/golang/go-mutex-semaphore/

// CreateImageT 이건 컨테이너 전용- 이미지 생성할때 고루틴 돌리니 에러 발생..
// TODO check ContainerCmd
func (dag *Dag) CreateImageT(ctx context.Context, healthChecker string) {

	if dag.ContainerCmd == nil {
		panic("ContainerCmd is not set")
	}

	eg, _ := errgroup.WithContext(ctx)

	for _, v := range dag.nodes {
		eg.Go(func() error {
			err := dag.ContainerCmd.CreateImage(v, healthChecker)
			if err != nil {
				return nil
			}
			return err
		})
	}
	err := eg.Wait()
	if err != nil {
		panic(err)
	}
}

func (dag *Dag) CreateImage( /*ctx context.Context, */ healthChecker string) {

	if dag.ContainerCmd == nil {
		panic("ContainerCmd is not set")
	}

	for _, v := range dag.nodes {
		err := dag.ContainerCmd.CreateImage(v, healthChecker)
		if err != nil {
			panic(err)
		}
	}
}

// Deprecated: Use edgeExists instead.
// findEdges returns -1 if an edge with the same parentId and childId exists, or 0 if not.
func findEdges(es []*Edge, parentId, childId string) int {
	for _, e := range es {
		if e.parentId == parentId && e.childId == childId {
			return -1
		}
	}
	return 0
}

/*func connectRunner_old250309(n *Node) {
	n.runner = func(ctx context.Context, n *Node, result chan *printStatus) {
		defer close(result)
		r := preFlight(ctx, n)
		result <- r
		r = inFlight(n)
		result <- r
		r = postFlight(n)
		result <- r
	}
}*/

// 자식 노드가 서로 다른 노드가 들어감. 데이터 내용은 같으나, 포인터가 다르다.
// visit 은 node.Id 로 방문 기록을 하고,
// ns 의 경우는 만약 자식노드가 생성이 되었고(방문이되으면) ns 에서 방문한(생성한) 자식노드를 넣어준다.
// 만약 circle 이면 무한루프 돔. (1,3), (3,1)
// visited 를 분리 해야 할까??
// cycle 이면 true, cycle 이 아니면 false
// deprecated cloneGraph
func cloneGraph(ns map[string]*Node) (map[string]*Node, bool) {

	if ns == nil {
		return nil, false
	}

	var (
		counter     = 0
		numElements = len(ns)
		iscycle     = false
	)

	visited := make(map[string]*Node, len(ns))

	var _cloneGraph func(node *Node, visited map[string]*Node) (*Node, bool)
	_cloneGraph = func(node *Node, visited map[string]*Node) (*Node, bool) {
		var (
			dupe  *Node
			cycle = false
		)

		if node == nil {
			return nil, false
		}

		n := visited[node.Id]
		// 여기서 cycle 을 잡을 수 있다.
		counter++
		// TODO 여기서 부터 220121
		if n != nil {
			dupe = n
			return dupe, false
		} else {

			if counter > numElements {
				return nil, true
			}
			dupe = new(Node)
			// 아래에도 있고 위에도 있음. 잘 이해해야함
			visited[node.Id] = dupe

			if node.children == nil {
				dupe.Id = node.Id
				//dupe.iterId = node.iterId
				dupe.children = nil
				dupe.parentDag = node.parentDag

			} else { // node.children 이 nil 이 아닐 경우
				dupe.children = make([]*Node, len(node.children))
				for i, ch := range node.children { // TODO 테스트 진행해야 한다.
					dupe.children[i], cycle = _cloneGraph(ch, visited)
				}
				// 부모의 정보를 입력한다.
				dupe.Id = node.Id
				//dupe.iterId = node.iterId
				dupe.parentDag = node.parentDag
			}

			visited[dupe.Id] = dupe
		}

		return dupe, cycle
	}

	for _, v := range ns {
		// _cloneGraph 실행될때 마다 visited 는 추가됨.
		_, iscycle = _cloneGraph(v, visited)
		if iscycle {
			break
		}
	}

	return visited, iscycle
}

// (do not erase) 참고 자료용
/*func preFlight(n *Node) *printStatus {

	if n == nil {
		return &printStatus{PreflightFailed, noNodeId}
	}
	i := len(n.parentVertex) // 부모 채널의 수
	wg := new(sync.WaitGroup)
	for j := 0; j < i; j++ {
		wg.Add(1)
		go func(c chan int) {
			defer wg.Done()
			<-c
			//close(c) postFlight 에서 close 해줌.
		}(n.parentVertex[j])
	}
	wg.Wait() // 모든 고루틴이 끝날 때까지 기다림

	return &printStatus{Preflight, n.Id}
}*/

```

```aiignore
// parser.go

package dag_go

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
)

// TODO xml 에서 nodes id 가 있어야 하고, pipeline id 와 매핑 되고,
// dag ID 는 uuid 인데, 이것은 xml 에서 데이터와 연계 될때 생성되는 uuid 값을 가지고 와서 매핑 된다.
// TODO xmlNode 를 별도로 두었지만, Node 로 통합하자.
// Nodes 를 어디에 둘지 고민..
// xml 이 수정될때 마다 수정해야 함으로 어느정도 완성된다음에 코드 수정 및 정리를 하자.

func xmlParser(x []*Node) (context.Context, bool, *Dag) {

	if x == nil {
		return nil, false, nil
	}

	n := len(x)
	//TODO 일단 에러 때문에 이렇게 넣었지만, 이걸 외부로 빼야함.
	//runnable := Connect()
	dag := NewDag()

	if n >= 1 { // node 가 최소 하나 이상은 있어야 한다.
		//dag := NewDag()
		// 순서데로 들어가기 때문에 for range 보다 유리함.
		for i := 0; i < n; i++ {
			no := x[i]

			// from 이 없으면 root 임.
			rn := len(no.from)
			if rn == 0 {
				dag.AddNodeToStartNode(no)
			}
			// 자신이 from 이므로, to 만 신경쓰면 된다.
			for _, v := range no.to {
				from := findNode(x, v)
				dag.AddEdge(no.Id, from.Id)
			}
		}

		//visited := dag.visitReset()
		//dag.detectCycle(dag.StartNode.Id, dag.StartNode.Id, visited)
		//result, _ := dag.detectCycle(dag.startNode.Id, dag.startNode.Id, visited)
		//fmt.Printf("%t", result)

		// 테스트 용도로 일단 넣어둠.
		dag.FinishDag()

		ctx := context.Background()
		dag.ConnectRunner()
		//dag.DagSetFunc()
		dag.GetReady(ctx)
		//dag.start()

		//b := dag.Wait(nil)
		//fmt.Println("모든 고루틴이 종료될때까지 그냥 기다림.")
		//fmt.Printf("true 이면 정상 : %t", b)

		return ctx, true, dag
	}

	return nil, false, nil
}

// TODO 함수들 정리해서 놓자.

func xmlProcess(parser *xml.Decoder) (int, []*Node) {
	var (
		counter         = 0
		n       *Node   = nil
		ns      []*Node = nil
		// TODO bool 로 하면 안됨 int 로 바꿔야 함. 초기 값은 0, false = 1, true = 2 로
		xStart    = false
		nStart    = false
		cmdStart  = false
		fromStart = false
		toStart   = false
	)
	// TODO error 처리 해줘야 함.
	if parser == nil {
		return 0, nil
	}

	for {
		token, err := parser.Token()

		// TODO 아래 두 if 구문 향후 수정하자.
		if err == io.EOF {
			break // TODO break 구문 수정 처리 필요.
		}

		// TODO 중복되는 것 같지만 일단 그냥 넣어둠.
		if token == nil {
			break
		}

		switch t := token.(type) {
		case xml.StartElement:
			//elem := xml.StartElement(t)
			xmlTag := t.Name.Local
			if xmlTag == nodes {
				xStart = true
			}
			if xmlTag == node {
				nStart = true

				n = new(Node)
				// node id 가 없으면 error 임.
				// 현재는 node 의 경우 속성이 1 이도록 강제함. 추후 수정할 필요가 있으면 수정.
				if len(t.Attr) != 1 {
					fmt.Println("node id 가 없음")
					return 0, nil
				}

				// TODO id 가 아니면 error	- strict 하지만 일단 이렇게
				if t.Attr[0].Name.Local == id {
					for _, v := range ns {
						if v.Id == t.Attr[0].Value {
							fmt.Println("중복된 node Id 존재")
							return 0, nil
						}
					}
					n.Id = t.Attr[0].Value
				}
			}

			if xmlTag == command {
				cmdStart = true
			}

			if xmlTag == from {
				fromStart = true
			}

			if xmlTag == to {
				toStart = true
			}

			counter++
		case xml.EndElement:
			//elem := xml.EndElement(t)
			xmlTag := t.Name.Local
			if xmlTag == nodes {
				if xStart {
					xStart = false
				} else {
					fmt.Println("error") // TODO error 처리 해줘야 함.
				}
			}
			// TODO 중복 구문들 function 으로 만들자.
			if xmlTag == node {
				if xStart { // StartElement 에서 true 해줌
					if nStart { // StartElement 에서 true 해줌
						if n != nil { // TODO nil 일 경우는 에러 처리 해줘야 함.
							ns = append(ns, n)
							nStart = false
							n = nil
						}
					}
				}
			}

			if xmlTag == from {
				if xStart {
					if nStart {
						if n != nil {
							fromStart = false
						}
					}
				}
			}

			if xmlTag == to {
				if xStart {
					if nStart {
						if n != nil {
							toStart = false
						}
					}
				}
			}

			if xmlTag == command {
				if xStart {
					if nStart {
						if cmdStart {
							cmdStart = false
						}
					}
				}
			}
			counter++

		case xml.CharData:
			if nStart {
				if n != nil {
					if cmdStart {
						// TODO string converting 바꾸기
						n.Commands = string(t)
					}
					if fromStart {
						n.from = append(n.from, string(t))
					}
					if toStart {
						n.to = append(n.to, string(t))
					}
				}
			}
		}
	}

	// TODO  일단 이렇게 그냥 해둠.
	return counter, ns
}

func newDecoder(b []byte) *xml.Decoder {
	// (do not erase) NewDecoder 에서 Strict field true 해줌.
	d := xml.NewDecoder(bytes.NewReader(b))
	return d
}

func XmlParser(d []byte) (context.Context, bool, *Dag) {

	decoder := newDecoder(d)
	_, nodes := xmlProcess(decoder)
	ctx, b, dag := xmlParser(nodes)

	return ctx, b, dag
}

```

```
//parser_test.go

package dag_go

import (
	"fmt"
	"testing"
)

func TestXmlProcess(t *testing.T) {
	d := serveXml()
	decoder := newDecoder(d)
	c, nodes := xmlProcess(decoder)

	fmt.Println("count :", c)

	for _, node := range nodes {
		fmt.Println("Node Id: ", node.Id)
		for _, t := range node.to {
			fmt.Println("To", t)
		}
		for _, f := range node.from {
			fmt.Println("From", f)
		}
		fmt.Println("Command ", node.Commands)

	}
}

func TestXmlsProcess(t *testing.T) {
	xmls := serveXmls()
	num := len(xmls)
	if num == 0 {
		fmt.Println("값이 없음")
		return
	}

	for _, xml := range xmls {
		d := []byte(xml)

		decoder := newDecoder(d)
		c, nodes := xmlProcess(decoder)

		fmt.Println("count :", c)

		for _, node := range nodes {
			fmt.Println("Node Id: ", node.Id)
			for _, t := range node.to {
				fmt.Println("To", t)
			}
			for _, f := range node.from {
				fmt.Println("From", f)
			}
			fmt.Println("Command ", node.Commands)

		}
	}

}

// 여러 형태의 xml 테스트 필요, 깨진 xml 로도 테스트 진행 필요.
// TODO nodes id 를 고유하게 만들어줘서, 이걸 dag id 로 넣어주자.
func serveXml() []byte {
	// id, to, from, command
	// id 는 attribute, to, from, command 는 tag
	// to, from 은 복수 가능.
	// from 이 없으면 시작노드, 파싱된 후에 start_node, end_node 추가 됨.
	xml := `
	<nodes>
		<node id = "1">
			<to>2</to>
			<command> echo "hello world 1"</command>
		</node>
		<node id = "2" >
			<from>1</from>
			<to>3</to>
			<to>8</to>
			<command>echo "hello world 2"</command>
		</node>
		<node id ="3" >
			<from>2</from>
			<to>4</to>
			<to>5</to>
			<command>echo "hello world 3"</command>
		</node>
		<node id ="4" >
			<from>3</from>
			<to>6</to>
			<command>echo "hello world 4"</command>
		</node>
		<node id ="5" >
			<from>3</from>
			<to>6</to>
			<command>echo "hello world 5"</command>
		</node>
		<node id ="6" >
			<from>4</from>
			<from>5</from>
			<to>7</to>
			<command>echo "hello world 6"</command>
		</node>
		<node id ="7" >
			<from>6</from>
			<to>9</to>
			<to>10</to>
			<command>echo "hello world 7"</command>
		</node>
		<node id ="8" >
			<from>2</from>
			<command>echo "hello world 8"</command>
		</node>
		<node id ="9" >
			<from>7</from>
			<to>11</to>
			<command>echo "hello world 9"</command>
		</node>
		<node id ="10" >
			<from>7</from>
			<to>11</to>
			<command>echo "hello world 10"</command>
		</node>
		<node id ="11" >
			<from>9</from>
			<from>10</from>
			<command>echo "hello world 11"</command>
		</node>
	</nodes>`

	return []byte(xml)
}

func serveXmls() []string {
	var xs []string

	// nodes 가 없는 경우
	failedXml3 := `
		<node id = "1" >
			<to>2</to>
			<command> echo "hello world 1"</command>
		</node>
		<node id = "2" >
			<from>1</from>
			<to>3</to>
			<command>echo "hello world 2"</command>
		</node>
		<node id ="3" >
			<from>2</from>
			<command>echo "hello world 3"</command>
		</node>`

	//xs = append(xs, failedXml1)
	//xs = append(xs, failedXml2)
	xs = append(xs, failedXml3)
	//xs = append(xs, failedXml4)

	return xs
}

func xmlss() {
	// id 가 없는 node
	failedXml1 := `
	<nodes>
		<node>
			<to>2</to>
			<command> echo "hello world 1"</command>
		</node>
		<node id = "2" >
			<from>1</from>
			<to>3</to>
			<command>echo "hello world 2"</command>
		</node>
		<node id ="3" >
			<from>2</from>
			<command>echo "hello world 3"</command>
		</node>
	</nodes>`

	// node id 가 중복된 경우
	failedXml2 := `
	<nodes>
		<node id = "2" >
			<to>2</to>
			<command> echo "hello world 1"</command>
		</node>
		<node id = "2" >
			<from>1</from>
			<to>3</to>
			<command>echo "hello world 2"</command>
		</node>
		<node id ="3" >
			<from>2</from>
			<command>echo "hello world 3"</command>
		</node>
	</nodes>`

	// nodes 가 없는 경우
	failedXml3 := `
		<node id = "1" >
			<to>2</to>
			<command> echo "hello world 1"</command>
		</node>
		<node id = "2" >
			<from>1</from>
			<to>3</to>
			<command>echo "hello world 2"</command>
		</node>
		<node id ="3" >
			<from>2</from>
			<command>echo "hello world 3"</command>
		</node>`

	// circle TODO 다수 테스트 진행해야함.
	failedXml4 := `
	<nodes>
		<node id = "1">
			<to>2</to>
			<command> echo "hello world 1"</command>
		</node>
		<node id = "2" >
			<from>1</from>
			<to>3</to>
			<to>1</to>
			<command>echo "hello world 2"</command>
		</node>
		<node id ="3" >
			<from>2</from>
			<command>echo "hello world 3"</command>
		</node>
	</nodes>`

	fmt.Println(failedXml1)
	fmt.Println(failedXml2)
	fmt.Println(failedXml3)
	fmt.Println(failedXml4)
}

// parser 테스트
// TODO 버그 있음. read |0: file already closed
// xmlParser 에서 입력 파라미터 포인터로 할지 생각하자.
// 순서대로 출력되는지 파악해야 한다. 잘못 출력되는 경우 발견.
func TestXmlParser(t *testing.T) {
	d := serveXml()
	decoder := newDecoder(d)
	_, nodes := xmlProcess(decoder)

	ctx, b, dag := xmlParser(nodes)

	if b {
		dag.Start()
		dag.Wait(ctx)
	}

}

```

```aiignore
// pipeline.go

package dag_go

import (
	"context"
	"fmt"
	"github.com/seoyhaein/utils"
	"time"

	"github.com/google/uuid"
)

type Pipeline struct {
	Id   string
	Dags []*Dag

	ContainerCmd Runnable
}

// NewPipeline  파이프라인은 dag NewPipeline와 데이터를 연계해야 하는데 데이터의 경우는 다른 xml 처리하는 것이 바람직할 것이다.
// 외부에서 데이터를 가지고 올 경우, ftp 나 scp 나 기타 다른 프롤토콜을 사용할 경우도 생각을 해야 한다.
func NewPipeline() *Pipeline {

	return &Pipeline{
		Id: uuid.NewString(),
	}
}

// Start TODO 모든 dag 들을 실행 시킬 수 있어야 한다. 수정해줘야 한다
func (pipe *Pipeline) Start(ctx context.Context) {
	if pipe.Dags == nil {
		return
	}

	for i, d := range pipe.Dags {
		d.ConnectRunner()
		//d.DagSetFunc()
		d.GetReady(ctx)
		d.Start()
		d.Wait(ctx)
		fmt.Println("count:", i)
	}
}

// Stop 개발 중
func (pipe *Pipeline) Stop(ctx context.Context, dag *Dag) {
	time.After(time.Second * 2)
}

// ReStart 개발 중
func (pipe *Pipeline) ReStart(ctx context.Context, dag *Dag) {

}

// NewDags 파이프라인과 dag 의 차이점은 데이터의 차이이다.
// 즉, 같은 dag 이지만 데이터가 다를 수 있다.
// 파이프라인에서 데이터 연계가 일어난다.
// 하지만 데이터 관련 datakit 이 아직 만들어 지지 않았기 때문에 입력파라미터로 dag 수를 지정한다.
// 이부분에서 두가지를 생각할 수 있다. dag 하나를 받아들여서 늘리는 방향과 dag 는 하나이고 데이터만큼만 어떠한 방식으로 진행하는 것이다.
// 전자가 쉽게 생각할 수 있지만 메모리 낭비 가있다. 일단 전자로 개발한다. 후자는 아직 아이디어가 없다.
// TODO 데이터와 관련해서 추가 해서 수정해줘야 한다. 추후 안정화 되면 panic 은 error 로 교체한다.
func (pipe *Pipeline) NewDags(ds int, original *Dag) *Pipeline {
	var dag *Dag

	if utils.IsEmptyString(pipe.Id) {
		panic("pipeline id is empty")
	}

	if ds < 1 {
		panic("input parameter is invalid")
	}

	for i := 1; i <= ds; i++ {
		dagId := fmt.Sprintf("%s-%d", pipe.Id, i)
		dag = CopyDag(original, dagId)

		if dag == nil {
			panic("CopyDag failed")
		}

		pipe.Dags = append(pipe.Dags, dag)
	}
	return pipe
}

func (pipe *Pipeline) SetContainerCmd(r Runnable) error {
	if r == nil {
		return fmt.Errorf("runnable is nil")
	}
	if pipe.ContainerCmd == nil {
		pipe.ContainerCmd = r
	}
	return nil
}

```

```aiignore
// pipeline_test.go

package dag_go

import (
	"context"
	"testing"
)

func TestNewPipeline(t *testing.T) {
	d := NewDag()
	d.AddEdge(d.StartNode.Id, "1")
	d.AddEdge("1", "2")
	d.AddEdge("1", "3")
	d.AddEdge("1", "4")
	d.AddEdge("2", "5")
	d.AddEdge("5", "6")

	/*d.AddCommand("1", `sleep 1`)
	d.AddCommand("2", `sleep 1`)
	d.AddCommand("3", `sleep 1`)
	d.AddCommand("4", `sleep 1`)
	d.AddCommand("5", `sleep 1`)
	d.AddCommand("6", `sleep 1`)*/

	err := d.FinishDag()
	if err != nil {
		panic(err)
	}
	//TODO 일단 구현에서 빠진 부분을 일단 보완하고 추가적으로 진행한다.
	copied := CopyDag(d, "78")
	ctx := context.Background()
	copied.ConnectRunner()
	//copy.DagSetFunc()
	copied.GetReady(ctx)
	copied.Start()
	//time.Sleep(time.Second * 10)
	copied.Wait(ctx)

	//copy := CopyDag(d)
	/*ctx := context.Background()
	d.DagSetFunc()
	d.GetReady(ctx)
	d.Start()
	d.Wait(ctx)*/
}

// 오류있어서 일단 테스트 해봐야 함.
func TestNewPipeline01(t *testing.T) {
	d := NewDag()
	d.AddEdge(d.StartNode.Id, "1")
	d.AddEdge("1", "2")
	d.AddEdge("1", "3")
	d.AddEdge("1", "4")
	d.AddEdge("2", "5")
	d.AddEdge("5", "6")

	/*d.AddCommand("1", `sleep 1`)
	d.AddCommand("2", `sleep 1`)
	d.AddCommand("3", `sleep 1`)
	d.AddCommand("4", `sleep 1`)
	d.AddCommand("5", `sleep 1`)
	d.AddCommand("6", `sleep 1`)*/

	err := d.FinishDag()
	if err != nil {
		panic(err)
	}
	//TODO 일단 구현에서 빠진 부분을 일단 보완하고 추가적으로 진행한다.

	pipe := NewPipeline()
	pipe.NewDags(1000, d)
	pipe.Start(context.Background())

	/*copy := CopyDag(d, "78")
	ctx := context.Background()
	copy.DagSetFunc()
	copy.GetReady(ctx)
	copy.Start()
	//time.Sleep(time.Second * 10)
	copy.Wait(ctx)*/

	//copy := CopyDag(d)
	/*ctx := context.Background()
	d.DagSetFunc()
	d.GetReady(ctx)
	d.Start()
	d.Wait(ctx)*/
}

// TODO 추후에 개선하자.
// https://www.practical-go-lessons.com/chap-34-benchmarks
func BenchmarkStart(b *testing.B) {
	for i := 0; i < b.N; i++ {
		demo(1)
	}
}

func demo(i int) {
	d := NewDag()
	d.AddEdge(d.StartNode.Id, "1")
	d.AddEdge("1", "2")
	d.AddEdge("1", "3")
	d.AddEdge("1", "4")
	d.AddEdge("2", "5")
	d.AddEdge("5", "6")
	err := d.FinishDag()
	if err != nil {
		panic(err)
	}
	pipe := NewPipeline()
	pipe.NewDags(i, d)
	pipe.Start(context.Background())
}




// Deprecated : 테스트 완료.
// preFlight_old_250306
func preFlight_old_250306(ctx context.Context, n *Node) *printStatus {
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
		Log.Println("Preflight", n.Id)
		return &printStatus{Preflight, n.Id}
	}
	n.succeed = false
	Log.Println("PreflightFailed", n.Id)
	return &printStatus{PreflightFailed, noNodeId}
}

func BenchmarkPreFlight_old_250306(b *testing.B) {
	// 로그가 있을 경우 출력결과물에 로그 기록이 남겨지는 것을 방지.
	Log.SetOutput(io.Discard)
	ctx := context.Background()
	// 예를 들어 부모 채널이 10개인 노드, 모두 Succeed 를 보내도록 설정
	node := setupNode("benchmark_preFlightCombined", 10, Succeed)

	// Warm-up: 한 번 실행 후 부모 채널 재채움
	_ = preFlight_old_250306(ctx, node)
	for j := 0; j < len(node.parentVertex); j++ {
		node.parentVertex[j] <- Succeed
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = preFlight_old_250306(ctx, node)
		// 매 반복 후 부모 채널 재채움
		for j := 0; j < len(node.parentVertex); j++ {
			node.parentVertex[j] <- Succeed
		}
	}
}


// closeChannels safely closes all channels in the DAG.
func (dag *Dag) closeChannels() {
	for _, edge := range dag.Edges {
		if edge.safeVertex != nil {
			edge.safeVertex.Close()
		}
	}

/*	// 각 노드에서 childrenVertex 와 parentVertex 채널들을 닫음.
	for _, nd := range dag.nodes {
		if nd == nil {
			continue
		}
		// childrenVertex 채널 닫기
		for _, ch := range nd.childrenVertex {
			if ch != nil {
				// 이미 닫혔을 가능성을 방어적으로 처리
				func(c chan runningStatus) {
					defer func() {
						if r := recover(); r != nil {
							// 이미 닫힌 채널일 경우 recover 하여 무시 또는 로깅
							Log.Warnf("recover from closing children channel: %v", r)
						}
					}()
					// 여기서 닫힌 채널을 닫을려고 하면 panic 이 발생함. 이거슬 위의 recover() 메서드에서 잡아줌.
					close(c)
				}(ch)
			}
		}

		// parentVertex 채널 닫기
		for _, ch := range nd.parentVertex {
			if ch != nil {
				func(c chan runningStatus) {
					defer func() {
						if r := recover(); r != nil {
							Log.Warnf("recover from closing parent channel: %v", r)
						}
					}()
					// 여기서 닫힌 채널을 닫을려고 하면 panic 이 발생함. 이거슬 위의 recover() 메서드에서 잡아줌.
					close(c)
				}(ch)
			}
		}
	}*/
}

// GetReady prepares the DAG for execution with separate handling for start and end nodes.
func (dag *Dag) GetReady(ctx context.Context) bool {
	dag.mu.Lock()
	defer dag.mu.Unlock()

	n := len(dag.nodes)
	if n < 1 {
		return false
	}

	// 워커 풀 초기화
	maxWorkers := min(n, dag.Config.WorkerPoolSize)
	dag.workerPool = NewDagWorkerPool(maxWorkers)

	var safeChs []*SafeChannel[*printStatus]
	// 종료 노드용 SafeChannel 생성
	endCh := NewSafeChannelGen[*printStatus](dag.Config.MinChannelBuffer)
	var end *Node

	// 각 노드를 순회하며 SafeChannel 생성 및 작업 제출
	for _, v := range dag.nodes {
		// 변수 캡처를 위해 별도 변수에 할당
		nd := v

		if nd.Id == StartNode {
			safeCh := NewSafeChannelGen[*printStatus](dag.Config.MinChannelBuffer)
			// 시작 노드는 맨 앞에 삽입
			safeChs = insertSafe(safeChs, 0, safeCh)

			// 워커 풀에 시작 노드의 작업 제출
			dag.workerPool.Submit(func() {
				select {
				case <-ctx.Done():
					safeCh.Close()
					return
				default:
					nd.runner(ctx, safeCh)
				}
			})
		}

		if nd.Id != StartNode && nd.Id != EndNode {
			safeCh := NewSafeChannelGen[*printStatus](dag.Config.MinChannelBuffer)
			safeChs = append(safeChs, safeCh)

			// 일반 노드 작업 제출
			dag.workerPool.Submit(func() {
				select {
				case <-ctx.Done():
					safeCh.Close()
					return
				default:
					nd.runner(ctx, safeCh)
				}
			})
		}

		if nd.Id == EndNode {
			end = nd
		}
	}

	if end == nil {
		Log.Println("Warning: EndNode is nil")
		return false
	}

	// 종료 노드 SafeChannel 를 safeChs 슬라이스에 추가
	safeChs = append(safeChs, endCh)

	// 종료 노드 작업 제출
	dag.workerPool.Submit(func() {
		select {
		case <-ctx.Done():
			endCh.Close()
			return
		default:
			end.runner(ctx, endCh)
		}
	})

	if dag.nodeResult != nil {
		return false
	}
	// 최종적으로 safeChs 슬라이스를 dag.runningStatus1에 저장
	dag.nodeResult = safeChs
	return true
}

func fanIn(ctx context.Context, channels []chan *printStatus) chan *printStatus {
	merged := make(chan *printStatus)
	var wg sync.WaitGroup

	// 각 입력 채널에 대한 고루틴 시작
	for _, ch := range channels {
		wg.Add(1)
		go func(c chan *printStatus) {
			defer wg.Done()
			for val := range c {
				select {
				case merged <- val:
				case <-ctx.Done():
					return
				}
			}
		}(ch)
	}

	// 모든 입력 채널이 닫히면 출력 채널도 닫음
	go func() {
		wg.Wait()
		close(merged)
	}()

	return merged
}

func insert(a []chan *printStatus, index int, value chan *printStatus) []chan *printStatus {
	if len(a) == index { // nil or empty slice or after last element
		return append(a, value)
	}
	a = append(a[:index+1], a[index:]...) // index < len(a)
	a[index] = value
	return a
}

func connectRunner(n *Node) {
	n.runner = func(ctx context.Context, result chan *printStatus) {
		defer close(result)

		// 헬퍼 함수: printStatus 값을 복사해 반환
		copyStatus := func(ps *printStatus) *printStatus {
			return &printStatus{
				rStatus: ps.rStatus,
				nodeId:  ps.nodeId,
			}
		}

		// 부모 노드 상태 확인
		if !n.CheckParentsStatus() {
			ps := newPrintStatus(PostFlightFailed, n.Id)
			// 복사본을 만들어 채널에 전달
			result <- copyStatus(ps)
			releasePrintStatus(ps)
			return
		}

		n.SetStatus(NodeStatusRunning)

		// preFlight 단계 실행
		ps := preFlight(ctx, n)
		result <- copyStatus(ps)
		if ps.rStatus == PreflightFailed {
			n.SetStatus(NodeStatusFailed)
			releasePrintStatus(ps)
			return
		}
		releasePrintStatus(ps)

		// inFlight 단계 실행
		ps = inFlight(n)
		result <- copyStatus(ps)
		if ps.rStatus == InFlightFailed {
			n.SetStatus(NodeStatusFailed)
			releasePrintStatus(ps)
			return
		}
		releasePrintStatus(ps)

		// postFlight 단계 실행
		ps = postFlight(n)
		result <- copyStatus(ps)
		if ps.rStatus == PostFlightFailed {
			n.SetStatus(NodeStatusFailed)
		} else {
			n.SetStatus(NodeStatusSucceeded)
		}
		releasePrintStatus(ps)
	}
}

func (dag *Dag) Wait(ctx context.Context) bool {
	// 채널 닫기는 DAG 종료 시 처리
	defer dag.closeChannels()

	// 워커 풀 종료
	if dag.workerPool != nil {
		defer dag.workerPool.Close()
	}

	// 컨텍스트에서 타임아웃 설정
	var waitCtx context.Context
	var cancel context.CancelFunc

	if dag.bTimeout {
		waitCtx, cancel = context.WithTimeout(ctx, dag.Timeout)
	} else {
		waitCtx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	// 채널 병합
	go dag.merge(waitCtx)

	for {
		select {
		case c := <-dag.RunningStatus1:
			if c.nodeId == EndNode {
				if c.rStatus == PreflightFailed {
					return false
				}
				if c.rStatus == InFlightFailed {
					return false
				}
				if c.rStatus == PostFlightFailed {
					return false
				}
				if c.rStatus == FlightEnd {
					return true
				}
			}
		case <-waitCtx.Done():
			Log.Printf("DAG execution timed out or cancelled: %v", waitCtx.Err())
			return false
		}
	}
}

func (dag *Dag) Start() bool {
    if len(dag.StartNode.parentVertex) != 1 {
        return false
    }
    dag.StartNode.SetSucceed(true)

    // Send 결과를 받을 채널을 만든다.
    resultCh := make(chan bool, 1)

    go func(sc *SafeChannel[runningStatus], resultCh chan bool) {
        success := sc.Send(Start)
        resultCh <- success // Send 의 결과를 resultCh에 전달한다.
        // 채널 닫기는 DAG 레벨에서 관리함.
    }(dag.StartNode.parentVertex[0], resultCh)

    // 고루틴에서 받은 결과를 기다린다.
    success := <-resultCh
    if !success {
        Log.Printf("Failed to send Start status on safe channel for start node")
        dag.StartNode.SetSucceed(false)
    }
    return success
}

func (dag *Dag) StartDag() (*Dag, error) {
	dag.mu.Lock()
	defer dag.mu.Unlock()

	if dag.StartNode = dag.createNode(StartNode); dag.StartNode == nil {
		return nil, fmt.Errorf("failed to create start node")
	}
	// 새 제네릭 SafeChannel 을 생성하고, 그 내부 채널을 시작 노드의 parentVertex 에 추가함.
	safeChan := NewSafeChannelGen[runningStatus](dag.Config.MinChannelBuffer)
	dag.StartNode.parentVertex = append(dag.StartNode.parentVertex, safeChan)
	return dag, nil
}


```