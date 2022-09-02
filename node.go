package dag_go

import (
	"context"
	"fmt"
	"github.com/seoyhaein/utils"
	"strings"

	"golang.org/x/sync/errgroup"
)

type Node struct {
	Id string
	// 컨테이너 빌드를 위한 from 이미지.
	ImageName string
	// TODO 이름 추후 수정하자.
	RunCommand Runnable

	children  []*Node // children
	parent    []*Node // parents
	parentDag *Dag    // 자신이 소속되어 있는 Dag
	Commands  string
	//status         string
	childrenVertex []chan runningStatus
	parentVertex   []chan runningStatus
	runner         func(ctx context.Context, n *Node, result chan *printStatus)
	// for xml parsing
	from []string
	to   []string
	// TODO re-thinking
	// https://yoongrammer.tistory.com/36
	//context.Context
	// 추후 commands string 과 교체
	//bashCommand []string
	// 추가
	succeed bool
}

// Debug 목적으로 스택? 두개 만들어서 채널에서 보내는 값과, 받는  값각각 넣어서 비교해본다.
// (do not erase) close 해주는 것 : func (dag *Dag) Wait(ctx context.Context) bool  에서 defer close(dag.RunningStatus) 해줌
// (do not erase) 너무 중요.@@@@ 채널 close 방식 확인하자. https://go101.org/article/channel-closing.html 너무 좋은 자료. 왜 제목을 101 이라고 했지 중급이상인데.
// setFunc commit by seoy
func setFunc(n *Node) {
	n.runner = func(ctx context.Context, n *Node, result chan *printStatus) {
		//(do not erase) defer close(result)
		r := preFlight(ctx, n)

		result <- r
		//TODO 특정 노드가 실패하면 여기서 빠져 나가야 할 것 같다.
		r = inFlight(n)
		result <- r
		//TODO 특정 노드가 실패하면 여기서 빠져 나가야 할 것 같다.
		r = postFlight(n)
		result <- r
		//TODO 특정 노드가 실패하면 여기서 빠져 나가야 할 것 같다.
	}
}

// preFlight preFlight, inFlight, postFlight 에서의 node 는 같은 노드이다.
// runner 에서 순차적으로 동작한다.
func preFlight(ctx context.Context, n *Node) *printStatus {
	if n == nil {
		panic(fmt.Errorf("node is nil"))
		//return &printStatus{PreflightFailed, noNodeId}
	}
	// 성공하면 context 사용한다.
	eg, _ := errgroup.WithContext(ctx)
	i := len(n.parentVertex) // 부모 채널의 수
	for j := 0; j < i; j++ {
		// (do not erase) 중요!! 여기서 들어갈 변수를 세팅않해주면 에러남.
		k := j
		c := n.parentVertex[k]
		eg.Go(func() error {
			// TODO 여기서 처리하자.
			result := <-c
			if result == Failed {
				return fmt.Errorf("failed")
			}
			return nil
		})
	}
	if err := eg.Wait(); err == nil { // 대기
		n.succeed = true
		return &printStatus{Preflight, n.Id}
	}
	n.succeed = false
	return &printStatus{PreflightFailed, noNodeId}
}

// (do not erase)
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

// inFlight preFlight, inFlight, postFlight 에서의 node 는 같은 노드이다.
// runner 에서 순차적으로 동작한다.
func inFlight(n *Node) *printStatus {
	if n == nil {
		panic(fmt.Errorf("node is nil"))
		//return &printStatus{InFlightFailed, noNodeId}
	}

	if n.Id == StartNode {
		Log.Println("start dag-go ", n.Id)
	}

	if n.Id == EndNode {
		Log.Println("end all tasks", n.Id)
	}
	//var bResult = false
	if n.Id == StartNode || n.Id == EndNode {
		//bResult = true
		n.succeed = true
	} else {
		// 성골할때만 명령을 실행시키고, 실패할경우는 채널에 값만 흘려 보낸다.
		// TODO 리턴 코드 작성하자.
		if n.succeed {
			r, err := n.Execute()
			Log.Println(n.Id, r)
			if err != nil {
				Log.Println("실패")
				//bResult = false
				n.succeed = false
			} else {
				n.succeed = true
			}
		}
	}
	if n.succeed {
		return &printStatus{InFlight, n.Id}
	} else {
		return &printStatus{InFlightFailed, n.Id}
	}
}

// postFlight preFlight, inFlight, postFlight 에서의 node 는 같은 노드이다.
// runner 에서 순차적으로 동작한다.
func postFlight(n *Node) *printStatus {
	if n == nil {
		panic(fmt.Errorf("node is nil"))
		//return &printStatus{PostFlightFailed, noNodeId}
	}

	if n.Id == EndNode {
		return &printStatus{FlightEnd, n.Id}
	}

	k := len(n.childrenVertex)
	if n.succeed {
		for j := 0; j < k; j++ {
			c := n.childrenVertex[j]
			c <- Succeed
			close(c)
		}
	} else {
		for j := 0; j < k; j++ {
			c := n.childrenVertex[j]
			c <- Failed
			close(c)
		}
	}
	return &printStatus{PostFlight, n.Id}
}

func checkVisit(visit map[string]bool) bool {
	for _, v := range visit {
		if v == false {
			return false
		}
	}
	return true
}

func getNode(s string, ns map[string]*Node) *Node {
	if len(strings.TrimSpace(s)) == 0 {
		return nil
	}

	size := len(ns)
	if size <= 0 {
		return nil
	}

	n := ns[s]
	return n
}

//getNextNode The first node to enter is fetched and the corresponding node is deleted.
func getNextNode(n *Node) *Node {

	if n == nil {
		return nil
	}
	if len(n.children) < 1 {
		return nil
	}

	ch := n.children[0]
	n.children = append(n.children[:0], n.children[1:]...)

	return ch
}

// 자식 노드가 서로 다른 노드가 들어감. 데이터 내용은 같으나, 포인터가 다르다.
// visit 은 node.Id 로 방문 기록을 하고,
// ns 의 경우는 만약 자식노드가 생성이 되었고(방문이되으면) ns 에서 방문한(생성한) 자식노드를 넣어준다.
// 만약 circle 이면 무한루프 돔. (1,3), (3,1)
// visited 를 분리 해야 할까??
// cycle 이면 true, cycle 이 아니면 false
// TODO 여기서 deepcopy 하는데 이거 정리하자. dag_test.go 에서 에러나는 거 찾아서 해결하자.
// cloneGraph
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

// CopyNodes 내일 테스트 해야함. dag.nodes 에 넣 어줬다. 이제 각각의 노드에 넣는 부분을 완성해야 한다.
func CopyNodes(original *Dag, copied *Dag) (map[string]*Node, bool) {
	num := len(original.nodes)
	if num < 1 {
		return nil, false
	}
	var nodes map[string]*Node
	// 1. 먼저 모든 노드들을 복사한다. (노드자체 가존재하지 않을 수 있으므로 부모 자식 아직 추가 않해줌.)
	for _, n := range original.nodes {
		node := new(Node)
		node.Id = n.Id
		node.ImageName = n.ImageName
		node.RunCommand = n.RunCommand
		//TODO parentDag 는 포인터 이기때문에 이거는 나중에 check 해줘야 함.
		node.parentDag = n.parentDag
		node.Commands = n.Commands
		node.runner = n.runner
		node.succeed = n.succeed

		nodes[copied.Id] = node
	}
	// 2. 부모 자식 노드들을 추가해줌.
	for _, n := range original.nodes {
		p := len(n.parent)
		ch := len(n.children)
		// 부모노드들을 추가한다.
		for i := 0; i > p; i++ {
			nodes[n.Id].parent = append(nodes[n.Id].parent, nodes[n.parent[i].Id])
		}
		// 자식 노드들을 추가한다.
		for i := 0; i > ch; i++ {
			nodes[n.Id].children = append(nodes[n.Id].children, nodes[n.children[i].Id])
		}
	}
	// 3. Vertex(channel) 을 복사한다. TODO CopyEdge 를 해준다음에 해야한다. 이러한 제약조건을 넣어주어야 한다. 일단 구현먼저.
	num = len(copied.Edges)
	if num < 1 {
		panic("edge is empty")
	}
	// TODO 내일 세부적으로 살펴보자. 채널을 임시로 수정해서(nodeId 넣고) 작성하고 완료 되면 수정된것을 지우자.
	// 각 노드에 vertex 넣는것은?? 헷갈린다.
	for _, e := range copied.Edges {
		nodes[e.parentId].parentVertex = append(nodes[e.parentId].parentVertex, e.vertex)
		nodes[e.childId].childrenVertex = append(nodes[e.childId].childrenVertex, e.vertex)
	}

	/*	for _, n := range original.nodes {
		pv := len(n.parentVertex)
		cv := len(n.childrenVertex)
		// parentVertex 만들어줌.
		for i := 0; i > pv; i++ {
			vertex := make(chan runningStatus, Min)
			nodes[n.Id].parentVertex = append(nodes[n.Id].parentVertex, vertex)
		}
		// childrenVertex 만들어줌.
		for i := 0; i > cv; i++ {
			vertex := make(chan runningStatus, Min)
			nodes[n.Id].childrenVertex = append(nodes[n.Id].childrenVertex, vertex)
		}
	}*/

	// 4. xml 을 위해 from, to 복사 해준다. (추후 없어질 수 있음.)
	for _, n := range original.nodes {
		for _, id := range n.from {
			nodes[n.Id].from = append(nodes[n.Id].from, id)
		}

		for _, id := range n.to {
			nodes[n.Id].to = append(nodes[n.Id].to, id)
		}
	}

	return nodes, true
}

/*
	Edges 			[]*Edge

	nodes     		map[string]*Node
	StartNode 		*Node
	EndNode   		*Node
	validated 		bool

	RunningStatus 	chan *printStatus

	// 에러를 모으는 용도.
	errLogs 		[]*systemError

	// timeout
	Timeout  		time.Duration
	bTimeout 		bool

	ContainerCmd 	Runnable

*/

/*
cm := new(Edge)
	cm.parentId = parentId
	cm.childId = childId
	cm.vertex = make(chan runningStatus, Min)
	dag.Edges = append(dag.Edges, cm)
*/

/*
dag.validated = false
	dag.StartNode = dag.createNode(StartNode)

	// 시작할때 넣어주는 채널 여기서 세팅된다.
	// error message : 중복된 node id 로 노드를 생성하려고 했습니다, createNode
	if dag.StartNode == nil {
		return nil
	}
	// 시작노드는 반드시 하나의 채널을 넣어 줘야 한다.
	// start 함수에서 채널 값을 넣어준다.
	dag.StartNode.parentVertex = append(dag.StartNode.parentVertex, make(chan runningStatus, Min))
	// TODO check
	dag.RunningStatus = make(chan *printStatus, Max)

*/

// CopyDag TODO 테스트 하자.
func CopyDag(original *Dag) (copied *Dag) {
	if original == nil {
		return nil
	}
	copied = &Dag{}
	// TODO check
	copied.Id = original.Id
	if utils.IsEmptyString(original.Pid) == false {
		copied.Pid = original.Pid
	}
	copied.Edges = CopyEdge(original.Edges)
	/*var edges int
	edges = len(original.Edges)
	// edge deep copy
	for i := 0; i < edges; i++ {
		e := new(Edge)
		e.parentId = original.Edges[i].parentId
		e.childId = original.Edges[i].childId
		e.vertex = make(chan runningStatus, Min)
		copied.Edges = append(copied.Edges, e)
	}*/

	// TODO cloneGraph 이번에 철저히 테스트 하자.
	// original nodes 의 startnode, endnode 수정해줘야 함.
	// 만약 성공적으로 deepcopy 가 되었다면 이 녀석을 찾아서 startNode 와 EndNode 에 넣자.
	// cloneGraph 에서 dag.nodes 가 복사가 되면서 그곳에서 channel 은 생성되었지만 edge(vertex) 는 생성되지 않음.
	ns, _ := cloneGraph(original.nodes)
	copied.nodes = ns

	// TODO StartNode, EndNode 설정 이건 테스트 해봐야 함.
	for _, n := range ns {
		if n.Id == StartNode {
			copied.StartNode = n
		}
		if n.Id == EndNode {
			copied.EndNode = n
		}
	}

	copied.validated = original.validated

	// TODO check 일단 주석처리함.
	//copied.RunningStatus = make(chan *printStatus, Max)

	// 생략 추후 넣어줌
	// 에러를 모으는 용도.
	//errLogs []*systemError

	copied.Timeout = original.Timeout
	copied.bTimeout = original.bTimeout

	copied.ContainerCmd = original.ContainerCmd

	return
}

func CopyEdge(original []*Edge) (copied []*Edge) {
	edges := len(original)
	if edges < 1 {
		return nil
	}
	for i := 0; i > edges; i++ {
		e := new(Edge)
		e.parentId = original[i].parentId
		e.childId = original[i].childId
		e.vertex = make(chan runningStatus, Min)

		copied[i] = e
	}
	return
}

// createNode add by seoy
func createNode(id string, r Runnable) (node *Node) {
	node = &Node{
		Id:         id,
		RunCommand: r,
	}
	return
}

func createNodeWithId(id string) (node *Node) {
	node = &Node{
		Id: id,
	}
	return
}

// Execute 이것을 작성하면 된다.
func (n *Node) Execute() (r int, err error) {
	if n.RunCommand != nil {
		r, err = execute(n)
		return
	}
	// Container 를 사용하 지않는 다른 명령어를 넣을 경우 여기서 작성하면 된다.
	return 7, nil
}

// execute add by seoy
func execute(this *Node) (int, error) {
	r, err := this.RunCommand.RunE(this)
	return r, err
}
