package dag_go

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/seoyhaein/utils"
	"golang.org/x/sync/errgroup"
)

type (
	runningStatus int

	printStatus struct {
		rStatus runningStatus
		nodeId  string
	}
)

// createEdgeErrorType 0 if created, 1 if exists, 2 if error.
type createEdgeErrorType int

const (
	Create createEdgeErrorType = iota
	Exist
	Fault
)

// The status displayed when running the runner on each node.
const (
	Start runningStatus = iota
	Preflight
	PreflightFailed
	InFlight
	InFlightFailed
	PostFlight
	PostFlightFailed
	FlightEnd
	Failed
	Succeed
)

const (
	nodes   = "nodes"
	node    = "node"
	id      = "id"
	from    = "from"
	to      = "to"
	command = "command"
)

const (
	StartNode = "start_node"
	EndNode   = "end_node"
)

// It is the node ID when the condition that the node cannot be created.
const noNodeId = "-1"

// channel buffer size
const (
	Max           int = 100
	Min           int = 1
	StatusDefault int = 3
)

// Dag (Directed Acyclic Graph) is an acyclic graph, not a cyclic graph.
// In other words, there is no cyclic cycle in the DAG algorithm, and it has only one direction.
type Dag struct {
	Pid   string
	Id    string
	Edges []*Edge

	nodes     map[string]*Node
	StartNode *Node
	EndNode   *Node
	validated bool

	RunningStatus chan *printStatus
	runningStatus []chan *printStatus

	// 에러를 모으는 용도.
	errLogs []*systemError

	// timeout
	Timeout  time.Duration
	bTimeout bool

	ContainerCmd Runnable
}

// Edge is a channel. It has the same meaning as the connecting line connecting the parent and child nodes.
type Edge struct {
	parentId string
	childId  string
	vertex   chan runningStatus
}

// NewDag creates a pointer to the Dag structure.
// One channel must be put in the start node. Enter this channel value in the start function.
// And this channel is not included in Edge.
// TODO 파라미터 nil 허용해주도록 바꿔줄지 생각함.
func NewDag() *Dag {
	dag := &Dag{
		nodes:         make(map[string]*Node),
		Id:            uuid.NewString(),
		RunningStatus: make(chan *printStatus, Max),
	}
	// TODO 이부분은 한번 생각해보자. 팩토리 메서드에서 아래 노드를 생성하고 채널을 추가하는게 맞는지, 이걸 따로 메서드로 빼는게 낫지 않을까?
	// StartNode 생성 및 검증
	if dag.StartNode = dag.createNode(StartNode); dag.StartNode == nil {
		return nil
	}
	// 시작 노드에 필수 채널 추가 (parentVertex 채널 삽입)
	dag.StartNode.parentVertex = append(dag.StartNode.parentVertex, make(chan runningStatus, Min))
	return dag
}

func (dag *Dag) SetContainerCmd(r Runnable) {
	dag.ContainerCmd = r
}

// createEdge creates an Edge. Edge has the ID of the child node and the ID of the child node.
// Therefore, the Edge is created with the ID of the parent node and the ID of the child node the same.
func (dag *Dag) createEdge(parentId, childId string) (*Edge, createEdgeErrorType) {
	if utils.IsEmptyString(parentId) || utils.IsEmptyString(childId) {
		return nil, Fault
	}

	// 이미 존재하는 엣지 확인
	if edgeExists(dag.Edges, parentId, childId) {
		return nil, Exist
	}

	edge := &Edge{
		parentId: parentId,
		childId:  childId,
		vertex:   make(chan runningStatus, Min),
	}

	dag.Edges = append(dag.Edges, edge)
	return edge, Create
}

func (dag *Dag) getVertex(parentId, childId string) chan runningStatus {

	for _, v := range dag.Edges {
		if v.parentId == parentId {
			if v.childId == childId {
				if v.vertex != nil {
					return v.vertex
				} else {
					return nil
				}
			}
		}
	}
	return nil
}

/*
	channel 은 일종의 edge 와 개념적으로 비슷하다.
	따라서, channel 의 수는 node 의 갯수와 밀접한 관련이 있다.
	이를 토대로 관련 메서드가 제작되었다.
*/
// TODO 생성된 edge 를 계산해본다.
func (dag *Dag) checkEdges() bool {
	return false
}

// createNode creates a pointer to a new node, but returns nil if dag has a duplicate node id.
func (dag *Dag) createNode(id string) *Node {
	// 이미 해당 id의 노드가 존재하면 nil 반환
	if _, exists := dag.nodes[id]; exists {
		return nil
	}
	var node *Node
	// TODO StartNode 와 EndNode 의 경우 ContainerCmd 이게 없을 수 있으므로 이렇게 한듯한데. 살펴보자.
	if dag.ContainerCmd != nil {
		node = createNode(id, dag.ContainerCmd)
	} else {
		node = createNodeWithId(id)
	}

	node.parentDag = dag
	dag.nodes[id] = node

	return node
}

// AddEdge error log 는 일단 여기서만 작성 TODO 로그를 기록하는 것을 활용할 방안 찾기, 또는 필요없으면 지우기.
func (dag *Dag) AddEdge(from, to string) error {
	// 에러 로그를 기록하고 반환하는 클로저 함수
	// TODO 이걸 빼서 메서드를 만들지 고민하자.
	logErr := func(err error) error {
		dag.errLogs = append(dag.errLogs, &systemError{AddEdge, err})
		return err
	}

	// 입력값 검증
	if from == to {
		return logErr(fmt.Errorf("from-node and to-node are same"))
	}
	if utils.IsEmptyString(from) {
		return logErr(fmt.Errorf("from-node is empty string"))
	}
	if utils.IsEmptyString(to) {
		return logErr(fmt.Errorf("to-node is empty string"))
	}

	// 노드를 가져오거나 생성하는 클로저 함수
	getOrCreateNode := func(id string) (*Node, error) {
		if node := dag.nodes[id]; node != nil {
			return node, nil
		}
		node := dag.createNode(id)
		if node == nil {
			return nil, logErr(fmt.Errorf("%s: createNode returned nil", id))
		}
		return node, nil
	}

	fromNode, err := getOrCreateNode(from)
	if err != nil {
		return err
	}
	toNode, err := getOrCreateNode(to)
	if err != nil {
		return err
	}

	// 자식과 부모 관계 설정
	fromNode.children = append(fromNode.children, toNode)
	toNode.parent = append(toNode.parent, fromNode)

	// 엣지 생성 및 검증
	edge, check := dag.createEdge(fromNode.Id, toNode.Id)
	if check == Fault || check == Exist {
		return logErr(fmt.Errorf("edge cannot be created"))
	}
	if edge == nil {
		return logErr(fmt.Errorf("vertex is nil"))
	}

	fromNode.childrenVertex = append(fromNode.childrenVertex, edge.vertex)
	toNode.parentVertex = append(toNode.parentVertex, edge.vertex)
	return nil
}

// TODO startNode 같은 경우는 NewDag 에서 만들어 줌. 이거 생각해봐야 함.
func (dag *Dag) addEndNode(fromNode, toNode *Node) error {
	// 입력 노드 검증
	if fromNode == nil {
		return fmt.Errorf("fromNode is nil")
	}
	if toNode == nil {
		return fmt.Errorf("toNode is nil")
	}

	// 부모-자식 관계 설정
	fromNode.children = append(fromNode.children, toNode)
	toNode.parent = append(toNode.parent, fromNode)

	// 엣지 생성 및 체크
	edge, check := dag.createEdge(fromNode.Id, toNode.Id)
	if check == Fault || check == Exist {
		return fmt.Errorf("edge cannot be created")
	}
	if edge == nil {
		return fmt.Errorf("vertex is nil")
	}

	// 엣지의 vertex 양쪽 노드에 추가
	fromNode.childrenVertex = append(fromNode.childrenVertex, edge.vertex)
	toNode.parentVertex = append(toNode.parentVertex, edge.vertex)

	return nil
}

// FinishDag finally, connect end_node to dag
func (dag *Dag) FinishDag() error {
	// 에러 로그를 기록하고 반환하는 클로저 함수 (finishDag 에러 타입 사용)
	logErr := func(err error) error {
		dag.errLogs = append(dag.errLogs, &systemError{finishDag, err})
		return err
	}

	// 이미 검증이 완료된 경우
	if dag.validated {
		return logErr(fmt.Errorf("validated is already set to true"))
	}

	// 노드가 하나도 없는 경우
	if len(dag.nodes) == 0 {
		return logErr(fmt.Errorf("no node"))
	}

	// 안전한 반복을 위해 노드 슬라이스를 생성 (맵의 구조 변경 방지를 위해)
	nodes := make([]*Node, 0, len(dag.nodes))
	for _, n := range dag.nodes {
		nodes = append(nodes, n)
	}

	// 종료 노드 생성 및 초기화
	dag.EndNode = dag.createNode(EndNode)
	if dag.EndNode == nil {
		return logErr(fmt.Errorf("failed to create end node"))
	}
	dag.EndNode.succeed = true

	// 각 노드에 대해 검증 및 종료 노드로의 연결 작업 수행
	for _, n := range nodes {
		// 부모와 자식이 없는 고립된 노드가 있는 경우
		if len(n.children) == 0 && len(n.parent) == 0 {
			if len(nodes) == 1 {
				// 노드가 단 하나일 경우, 반드시 시작 노드여야 함.
				if n.Id != StartNode {
					return logErr(fmt.Errorf("invalid node: only node is not the start node"))
				}
			} else {
				return logErr(fmt.Errorf("node '%s' has no parent and no children", n.Id))
			}
		}

		// 종료 노드가 아니면서 자식이 없는 경우, 종료 노드와 연결
		if n.Id != EndNode && len(n.children) == 0 {
			if err := dag.addEndNode(n, dag.EndNode); err != nil {
				return logErr(fmt.Errorf("addEndNode failed for node '%s': %w", n.Id, err))
			}
		}
	}

	// 검증 완료 플래그 설정
	dag.validated = true
	return nil
}

func (dag *Dag) visitReset() map[string]bool {

	size := len(dag.nodes)
	if size <= 0 {
		return nil
	}

	visited := make(map[string]bool, len(dag.nodes))

	for k := range dag.nodes {
		visited[k] = false
	}
	return visited
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

func (dag *Dag) ConnectRunner() bool {
	n := len(dag.nodes)
	if n < 1 {
		return false
	}
	for _, v := range dag.nodes {
		connectRunner(v)
	}
	return true
}

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

// TODO 정상 작동하면 channel 설정하자.
// https://jacking75.github.io/go_channel_howto/
func connectRunner(n *Node) {
	n.runner = func(ctx context.Context, n *Node, result chan *printStatus) {
		defer close(result)
		r := preFlight(ctx, n)
		result <- r
		r = inFlight(n)
		result <- r
		r = postFlight(n)
		result <- r
	}
}

// GetReadyT TODO 맨 마지막에 end 넣는 것은 생각해보자. 굳이 필요 없는 것 같긴하다.
func (dag *Dag) GetReadyT(ctx context.Context) bool {
	n := len(dag.nodes)
	if n < 1 {
		return false
	}
	var chs []chan *printStatus
	for _, v := range dag.nodes {
		ch := make(chan *printStatus, StatusDefault)
		//dag.runningStatus = append(dag.runningStatus, ch)
		chs = append(chs, ch)
		go v.runner(ctx, v, ch)
	}
	if dag.runningStatus != nil {
		return false
	}
	dag.runningStatus = chs
	return true
}

func (dag *Dag) GetReady(ctx context.Context) bool {
	n := len(dag.nodes)
	if n < 1 {
		return false
	}
	var chs []chan *printStatus
	endCh := make(chan *printStatus, StatusDefault)
	var end *Node
	for _, v := range dag.nodes {
		if v.Id == StartNode {
			start := make(chan *printStatus, StatusDefault)
			chs = insert(chs, 0, start)
			go v.runner(ctx, v, start)
		}

		if v.Id != StartNode && v.Id != EndNode {
			ch := make(chan *printStatus, StatusDefault)
			chs = append(chs, ch)
			go v.runner(ctx, v, ch)
		}

		if v.Id == EndNode {
			end = v
		}
	}

	if end == nil {
		panic("EndNode is nil")
	}
	chs = append(chs, endCh)
	go end.runner(ctx, end, endCh)

	if dag.runningStatus != nil {
		return false
	}
	dag.runningStatus = chs
	return true
}

// https://stackoverflow.com/questions/46128016/insert-a-value-in-a-slice-at-a-given-index
// insert 테스트 후 utils 에 넣기
func insert(a []chan *printStatus, index int, value chan *printStatus) []chan *printStatus {
	if len(a) == index { // nil or empty slice or after last element
		return append(a, value)
	}
	a = append(a[:index+1], a[index:]...) // index < len(a)
	a[index] = value
	return a
}

// Start start_node has one vertex. That is, it has only one channel and this channel is not included in the edge.
// It is started by sending a value to this channel when starting the dag's operation.
func (dag *Dag) Start() bool {
	n := len(dag.StartNode.parentVertex)
	// 1 이 아니면 에러다.
	if n != 1 {
		return false
	}

	dag.StartNode.succeed = true
	go func(c chan runningStatus) {
		ch := c
		ch <- Start
		close(ch)
	}(dag.StartNode.parentVertex[0])

	return true
}

// Wait waits until all channels are closed. RunningStatus channel has multiple senders and one receiver
// Closing a channel on a receiver violates the general channel close principle.
// However, when Wait terminates, it seems safe to close the channel here because all tasks are finished.
func (dag *Dag) Wait(ctx context.Context) bool {
	//defer close(dag.RunningStatus)
	dag.mergeT()
	for {
		if dag.bTimeout {
			select {
			case c := <-dag.RunningStatus:
				//printRunningStatus(c)
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
			case <-time.After(dag.Timeout):
				return false
			case <-ctx.Done():
				return false
			}
		} else {
			select {
			case c := <-dag.RunningStatus:
				//printRunningStatus(c)
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
			case <-ctx.Done():
				return false
			}
		}
	}
}

// merge 잠재적인 버그인데... 먼저 도착한 것이 기다리는 상황이 발생함.
func (dag *Dag) merge() {
	defer close(dag.RunningStatus)

	n := len(dag.runningStatus)
	if n < 1 {
		return
	}
	for i := 0; i < n; i++ {
		ch := dag.runningStatus[i]
		for v := range ch {
			dag.RunningStatus <- v
		}
	}
}

// mergeT 테스트는 내일 하자.
func (dag *Dag) mergeT() {
	defer close(dag.RunningStatus)

	n := len(dag.runningStatus)

	var (
		order []chan *printStatus
		// 가장 빠른 채널을 확인할때 나오는 값을 받을 slice
		values []*printStatus
		added  []int

		j     = 0
		index = 0
	)
	for {
		// 첫번째 값은 제일 빠른 채널을 선택하는 기준이다.
		// 추가했으면 더 이상 추가 하면 안됨.
		//nnn := len(added)
		//if nnn > 0 {
		for _, a := range added {
			if j == a {
				j++
			}
		}
		if j < n {
			index = j

			ch := dag.runningStatus[index]
			select {
			case c := <-ch:
				order = append(order, ch)
				values = append(values, c)
				// 추가한 index 넣어줌.
				added = append(added, index)
				break
			default:
				break
			}
		}
		nn := len(order)

		if nn == n {
			break
		}

		if j == n-1 {
			j = 0
		}
	}
	no := len(order)
	nv := len(values)

	if no == nv {
		for i := 0; i < len(values); i++ {
			dag.RunningStatus <- values[i]

			for c := range order[i] {
				dag.RunningStatus <- c
			}
		}
	}
	// TODO error check!
	if no != nv {
		panic("error")
	}
}

// 테스트 용으로 만듬.
func (dag *Dag) debugLog() {
	if len(dag.errLogs) > 0 {

		for _, v := range dag.errLogs {
			Log.Printf(
				"error type: %d\n reaseon:%s\n",
				v.errorType, v.reason.Error(),
			)
		}
	}
}

// SetTimeout commit by seoy
func (dag *Dag) SetTimeout(d time.Duration) {
	if dag.bTimeout == false {
		dag.bTimeout = true
	}
	dag.Timeout = d
}

// DisableTimeout commit by seoy
func (dag *Dag) DisableTimeout() {
	if dag.bTimeout == true {
		dag.bTimeout = false
	}
}

// AddCommand add command to node.
func (dag *Dag) AddCommand(id, cmd string) (node *Node) {
	node = nil
	if n, b := nodeExist(dag, id); b == true {
		n.Commands = cmd
	}
	return
}

// AddNodeToStartNode check
// TODO 확인하자.
func (dag *Dag) AddNodeToStartNode(to *Node) error {

	if to == nil {
		return fmt.Errorf("node is nil")
	}

	fromNode := dag.StartNode
	toNode := dag.nodes[to.Id]

	if toNode != nil {
		return fmt.Errorf("duplicate nodes exist")
	}

	if fromNode == toNode {
		return fmt.Errorf("from-node and to-node are same")
	}

	toNode = to
	fromNode.children = append(fromNode.children, toNode)
	toNode.parent = append(toNode.parent, fromNode)

	dag.createEdge(fromNode.Id, toNode.Id)

	v := dag.getVertex(fromNode.Id, toNode.Id)

	if v != nil {
		fromNode.childrenVertex = append(fromNode.childrenVertex, v)
		toNode.parentVertex = append(toNode.parentVertex, v)
	} else {
		// TODO check
		Log.Println("error")
	}
	return nil
}

// copyDag dag 를 복사함.
func copyDag(original *Dag) (map[string]*Node, []*Edge) {
	num := len(original.nodes)
	if num < 1 {
		return nil, nil
	}
	var nodes map[string]*Node
	nodes = make(map[string]*Node, len(original.nodes))

	// 1. 먼저 모든 노드들을 복사한다. (노드자체 가존재하지 않을 수 있으므로 부모 자식 아직 추가 않해줌.)
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

func CopyEdge(original []*Edge) (copied []*Edge) {
	edges := len(original)
	if edges < 1 {
		return nil
	}
	copied = make([]*Edge, edges)
	for i := 0; i < edges; i++ {
		e := new(Edge)
		e.parentId = original[i].parentId
		e.childId = original[i].childId
		e.vertex = make(chan runningStatus, Min)

		copied[i] = e
	}
	return
}

// internal methods

func findNode(ns []*Node, id string) *Node {
	if ns == nil {
		return nil
	}
	for _, n := range ns {
		if n.Id == id {
			return n
		}
	}
	return nil
}

// nodeExist returns true if node is in dag, false otherwise
func nodeExist(dag *Dag, nodeId string) (*Node, bool) {
	for _, n := range dag.nodes {
		if n.Id == nodeId {
			return n, true
		}
	}
	return nil, false
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

func edgeExists(es []*Edge, parentId, childId string) bool {
	for _, e := range es {
		if e.parentId == parentId && e.childId == childId {
			return true
		}
	}
	return false
}

func findEdgeFromParentId(es []*Edge, Id string) []*Edge {
	var r []*Edge
	for _, e := range es {
		if e.parentId == Id {
			r = append(r, e)
		}
	}
	return r
}

func findEdgeFromChildId(es []*Edge, Id string) []*Edge {
	var r []*Edge
	for _, e := range es {
		if e.childId == Id {
			r = append(r, e)
		}
	}
	return r
}

// printRunningStatus TODO context cancel 관련 해서 추가 해줘야 하고 start() 같은 경우도 처리 해줘야 한다.
// TODO 나중에 수정해주자.
func printRunningStatus(status *printStatus) {
	var r string
	if status == nil {
		return
	}
	if status.rStatus == Start {
		r = "Start"
	}
	if status.rStatus == Preflight {
		r = "Preflight"
	}
	if status.rStatus == PreflightFailed {
		r = "PreflightFailed"
	}
	if status.rStatus == InFlight {
		r = "InFlight"
	}
	if status.rStatus == InFlightFailed {
		r = "InFlightFailed"
	}
	if status.rStatus == PostFlight {
		r = "PostFlight"
	}
	if status.rStatus == PostFlightFailed {
		r = "PostFlightFailed"
	}
	if status.rStatus == FlightEnd {
		r = "FlightEnd"
	}
	if status.rStatus == Failed {
		r = "Failed"
	}
	if status.rStatus == Succeed {
		r = "Succeed"
	}

	fmt.Printf("nodeId:%s, status:%s\n", status.nodeId, r)

}
