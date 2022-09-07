package dag_go

import (
	"context"
	"fmt"
	"golang.org/x/sync/errgroup"
	"time"

	"github.com/google/uuid"
	"github.com/seoyhaein/utils"
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

// TODO 함 수호출 순서 정하자. 추후 생각하자.
type callOrder struct {
}

// NewDag creates a pointer to the Dag structure.
// One channel must be put in the start node. Enter this channel value in the start function.
// And this channel is not included in Edge.
// TODO 파라미터 nil 허용해주도록 바꿔줄지 생각함.
func NewDag() *Dag {
	dag := new(Dag)
	dag.nodes = make(map[string]*Node)
	dag.Id = uuid.NewString()
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

	return dag
}

func NewDagWithPIdT(pid string, n string, r Runnable) *Dag {
	dag := new(Dag)
	dag.nodes = make(map[string]*Node)
	dag.Id = fmt.Sprintf("%s-%s", pid, n)
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

	return dag
}

func NewDagWithPId(dagId string, r Runnable) *Dag {
	dag := new(Dag)
	dag.nodes = make(map[string]*Node)
	dag.Id = dagId
	//dag.Id = fmt.Sprintf("%s-%s", pid, n)
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

	return dag
}

func (dag *Dag) SetContainerCmd(r Runnable) {
	dag.ContainerCmd = r
}

// createEdge creates an Edge. Edge has the ID of the child node and the ID of the child node.
// Therefore, the Edge is created with the ID of the parent node and the ID of the child node the same.
func (dag *Dag) createEdgeT(parentId, childId string) (*Edge, createEdgeErrorType) {
	if utils.IsEmptyString(parentId) || utils.IsEmptyString(childId) {
		return nil, Fault
	}
	// 총 4가지를 생각해야 할 거 같다.
	for _, v := range dag.Edges {
		if v.parentId == parentId {
			if v.childId == childId { // parentId 는 같고, childId 같을 경우
				return nil, Exist
			} else { // parentId 는 같고, childId 가 다를 경우
				cm := new(Edge)
				cm.parentId = v.parentId
				cm.childId = childId
				cm.vertex = make(chan runningStatus, Min)

				dag.Edges = append(dag.Edges, cm)

				return cm, Create
			}
		} else { // parentId 가 다를 경우
			if v.childId == childId { // childId 같을 경우
				cm := new(Edge)
				cm.parentId = parentId
				cm.childId = v.childId
				cm.vertex = make(chan runningStatus, Min)

				dag.Edges = append(dag.Edges, cm)

				return cm, Create
			} else { // parentId 가 다르고 childId 도 다를 경우
				cm := new(Edge)
				cm.parentId = parentId
				cm.childId = childId
				cm.vertex = make(chan runningStatus, Min)

				dag.Edges = append(dag.Edges, cm)

				return cm, Create
			}
		}
	}
	// range 구문이 안돌 경우, 데이터가 없을때.
	cm := new(Edge)
	cm.parentId = parentId
	cm.childId = childId
	cm.vertex = make(chan runningStatus, Min)

	dag.Edges = append(dag.Edges, cm)

	return cm, Create
}

func (dag *Dag) createEdge(parentId, childId string) (*Edge, createEdgeErrorType) {
	if utils.IsEmptyString(parentId) || utils.IsEmptyString(childId) {
		return nil, Fault
	}

	r := findEdges(dag.Edges, parentId, childId)

	if r == -1 {
		return nil, Exist
	}
	cm := new(Edge)
	cm.parentId = parentId
	cm.childId = childId
	cm.vertex = make(chan runningStatus, Min)
	dag.Edges = append(dag.Edges, cm)

	return cm, Create

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

	for _, n := range dag.nodes {
		if n.Id == id {
			return nil
		}
	}

	var node *Node

	if dag.ContainerCmd != nil {
		node = createNode(id, dag.ContainerCmd)
	} else {
		node = createNodeWithId(id)
	}

	node.parentDag = dag
	dag.nodes[id] = node

	return node
}

//AddEdge error log 는 일단 여기서만 작성
func (dag *Dag) AddEdge(from, to string) error {

	if from == to {
		err := fmt.Errorf("from-node and to-node are same")
		dag.errLogs = append(dag.errLogs, &systemError{AddEdge, err})
		return err
	}

	if utils.IsEmptyString(from) {
		err := fmt.Errorf("from-node is emtpy string")
		dag.errLogs = append(dag.errLogs, &systemError{AddEdge, err})
		return err
	}

	if utils.IsEmptyString(to) {
		err := fmt.Errorf("to-node is emtpy string")
		dag.errLogs = append(dag.errLogs, &systemError{AddEdge, err})
		return err
	}

	fromNode := dag.nodes[from]
	if fromNode == nil {
		fromNode = dag.createNode(from)

		if fromNode == nil {
			err := fmt.Errorf("fromNode: createNode return nil")
			dag.errLogs = append(dag.errLogs, &systemError{AddEdge, err})
			return err
		}
	}
	toNode := dag.nodes[to]
	if toNode == nil {
		toNode = dag.createNode(to)
		if toNode == nil {
			err := fmt.Errorf("toNode: createNode return nil")
			dag.errLogs = append(dag.errLogs, &systemError{AddEdge, err})
			return err
		}
	}

	fromNode.children = append(fromNode.children, toNode)
	toNode.parent = append(toNode.parent, fromNode)

	edge, check := dag.createEdge(fromNode.Id, toNode.Id)

	if check == Fault || check == Exist {
		err := fmt.Errorf("Edge cannot be created")
		dag.errLogs = append(dag.errLogs, &systemError{AddEdge, err})
		return err
	}

	if edge != nil {
		fromNode.childrenVertex = append(fromNode.childrenVertex, edge.vertex)
		toNode.parentVertex = append(toNode.parentVertex, edge.vertex)
	} else {
		err := fmt.Errorf("vertex is nil")
		dag.errLogs = append(dag.errLogs, &systemError{AddEdge, err})
		return err
	}
	return nil
}

func (dag *Dag) addEndNode(fromNode, toNode *Node) error {

	if fromNode == nil {
		return fmt.Errorf("fromeNode is nil")
	}
	if toNode == nil {
		return fmt.Errorf("toNode is nil")
	}

	fromNode.children = append(fromNode.children, toNode)
	toNode.parent = append(toNode.parent, fromNode)

	edge, check := dag.createEdge(fromNode.Id, toNode.Id)
	if check == Fault || check == Exist {
		return fmt.Errorf("Edge cannot be created")
	}
	//v := dag.getVertex(fromNode.Id, toNode.Id)

	if edge != nil {
		fromNode.childrenVertex = append(fromNode.childrenVertex, edge.vertex)
		toNode.parentVertex = append(toNode.parentVertex, edge.vertex)
	} else {
		fmt.Errorf("vertex is nil")
	}
	return nil
}

// FinishDag finally, connect end_node to dag
func (dag *Dag) FinishDag() error {

	if dag.validated {
		return fmt.Errorf("validated is already set to true")
	}

	if len(dag.nodes) == 0 {
		return fmt.Errorf("no node")
	}
	temp := make(map[string]*Node)
	for k, v := range dag.nodes {
		temp[k] = v
	}

	dag.EndNode = dag.createNode(EndNode)
	//TODO check add by seoy
	dag.EndNode.succeed = true
	for _, n := range temp {
		if len(n.children) == 0 && len(n.parent) == 0 {
			if len(temp) == 1 {
				if n.Id != StartNode {
					return fmt.Errorf("there is an invalid node")
				}
			} else {
				return fmt.Errorf("there are nodes that have no parent node and no child nodes")
			}
		}
		if n.Id != EndNode && len(n.children) == 0 {
			err := dag.addEndNode(n, dag.EndNode)
			if err != nil {
				return fmt.Errorf("addEndNode failed")
			}
		}
	}
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

func (dag *Dag) DagSetFunc() bool {
	n := len(dag.nodes)
	if n < 1 {
		return false
	}
	for _, v := range dag.nodes {
		setFunc(v)
	}
	return true
}

// BeforeGetReady 이건 컨테이너 전용- 이미지 생성할때 고루틴 돌리니 에러 발생..
// TODO check ContainerCmd
func (dag *Dag) BeforeGetReady(ctx context.Context, healthChecker string) {

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

func (dag *Dag) BeforeGetReadyT(ctx context.Context, healthChecker string) {

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

func (dag *Dag) GetReady(ctx context.Context) bool {
	n := len(dag.nodes)
	if n < 1 {
		return false
	}
	for _, v := range dag.nodes {
		go v.runner(ctx, v, dag.RunningStatus)
	}
	return true
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
	defer close(dag.RunningStatus)

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
				printRunningStatus(c)
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

// 테스트 용으로 만듬.
func (dag *Dag) debugLog() {
	if len(dag.errLogs) > 0 {

		for _, v := range dag.errLogs {
			log.Printf(
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

// AddCommand add command to node. TODO node 의 field 가 늘어날때 수정해준다.
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

// copyDag 내일 테스트 해야함. dag.nodes 에 넣 어줬다. 이제 각각의 노드에 넣는 부분을 완성해야 한다.
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

// CopyDag TODO 테스트 하자.
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

//findEdges 같은게 있으면 -1, 같은게 없으면 0
func findEdges(es []*Edge, parentId, childId string) int {
	for _, e := range es {
		if e.parentId == parentId && e.childId == childId {
			return -1
		}
	}
	return 0
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

//printRunningStatus TODO context cancel 관련 해서 추가 해줘야 하고 start() 같은 경우도 처리 해줘야 한다.
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
