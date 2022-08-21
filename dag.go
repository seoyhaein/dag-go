package dag_go

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/seoyhaein/utils"
)

// Dag (Directed Acyclic Graph) is an acyclic graph, not a cyclic graph.
// In other words, there is no cyclic cycle in the DAG algorithm, and it has only one direction.
type Dag struct {
	Id    string
	Edges []*Edge

	nodes     map[string]*Node
	startNode *Node
	endNode   *Node
	validated bool

	RunningStatus chan *printStatus

	// 에러를 모으는 용도.
	errLogs []*systemError

	// timeout
	Timeout  time.Duration
	bTimeout bool

	//cmd *Command

	// TODO 이름 추후 수정하자.
	RunCommand Runnable
}

// Edge is a channel. It has the same meaning as the connecting line connecting the parent and child nodes.
type Edge struct {
	parentId string
	childId  string
	vertex   chan int
}

// TODO 함 수호출 순서 정하자. 추후 생각하자.
type callOrder struct {
}

// NewDag creates a pointer to the Dag structure.
// One channel must be put in the start node. Enter this channel value in the start function.
// And this channel is not included in Edge.
func NewDag(r Runnable) *Dag {
	dag := new(Dag)
	dag.nodes = make(map[string]*Node)
	dag.Id = uuid.NewString()
	dag.validated = false
	dag.startNode = dag.createNode(StartNode)

	// 시작할때 넣어주는 채널 여기서 세팅된다.
	// error message : 중복된 node id 로 노드를 생성하려고 했습니다, createNode
	if dag.startNode == nil {
		return nil
	}
	// 시작노드는 반드시 하나의 채널을 넣어 줘야 한다.
	// start 함수에서 채널 값을 넣어준다.
	dag.startNode.parentVertex = append(dag.startNode.parentVertex, make(chan int, 1))

	// TODO 일단 퍼퍼를 1000 으로 둠
	// TODO n 을 향후에는 조정하자. 각 노드의 수로 채널 버퍼를 지정할 수 없다. 또한 그럴 필요도 없을 수도 있다.
	dag.RunningStatus = make(chan *printStatus, 1000)

	// TODO 일단 간단히 넣었음.
	dag.RunCommand = r

	return dag
}

func NewDagWithPId(pid string, n string, r Runnable) *Dag {
	dag := new(Dag)
	dag.nodes = make(map[string]*Node)
	dag.Id = fmt.Sprintf("%s-%s", pid, n)
	dag.validated = false

	dag.startNode = dag.createNode(StartNode)

	// 시작할때 넣어주는 채널 여기서 세팅된다.
	// error message : 중복된 node id 로 노드를 생성하려고 했습니다, createNode
	if dag.startNode == nil {
		return nil
	}
	// 시작노드는 반드시 하나의 채널을 넣어 줘야 한다.
	// start 함수에서 채널 값을 넣어준다.
	dag.startNode.parentVertex = append(dag.startNode.parentVertex, make(chan int, 1))

	// TODO 일단 퍼퍼를 1000 으로 둠
	// TODO n 을 향후에는 조정하자. 각 노드의 수로 채널 버퍼를 지정할 수 없다. 또한 그럴 필요도 없을 수도 있다.
	dag.RunningStatus = make(chan *printStatus, 1000)

	return dag
}

// createEdge creates an Edge. Edge has the ID of the child node and the ID of the child node.
// Therefore, the Edge is created with the ID of the parent node and the ID of the child node the same.
func (dag *Dag) createEdge(parentId, childId string) (*Edge, createEdgeErrorType) {
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
				cm.vertex = make(chan int, 1)

				dag.Edges = append(dag.Edges, cm)

				return cm, Create
			}
		} else { // parentId 가 다를 경우
			if v.childId == childId { // childId 같을 경우
				cm := new(Edge)
				cm.parentId = parentId
				cm.childId = v.childId
				cm.vertex = make(chan int, 1)

				dag.Edges = append(dag.Edges, cm)

				return cm, Create
			} else { // parentId 가 다르고 childId 도 다를 경우
				cm := new(Edge)
				cm.parentId = parentId
				cm.childId = childId
				cm.vertex = make(chan int, 1)

				dag.Edges = append(dag.Edges, cm)

				return cm, Create
			}
		}
	}
	// range 구문이 안돌 경우, 데이터가 없을때.
	cm := new(Edge)
	cm.parentId = parentId
	cm.childId = childId
	cm.vertex = make(chan int, 1)

	dag.Edges = append(dag.Edges, cm)

	return cm, Create
}

func (dag *Dag) getVertex(parentId, childId string) chan int {

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
	// TODO 추가적으로 수정할 예정임.
	node := createNode(id, dag.RunCommand)

	node.parentDag = dag
	dag.nodes[id] = node

	return node
}

// error log 는 일단 여기서만 작성

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

	dag.endNode = dag.createNode(EndNode)
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
		if n.Id != EndNode {
			err := dag.addEndNode(n, dag.endNode)
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

func (dag *Dag) DagSetFunc(ctx context.Context) bool {
	n := len(dag.nodes)
	if n < 1 {
		return false
	}
	// TODO setFunc 리턴을 추 가해줘야 함.
	for _, v := range dag.nodes {
		setFunc(ctx, v)
	}
	return true
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
	n := len(dag.startNode.parentVertex)
	// 1 이 아니면 에러다.
	if n != 1 {
		return false
	}

	go func(c chan int) {
		ch := c
		ch <- 1
		// add by seoy, channel closing principle
		close(ch)
	}(dag.startNode.parentVertex[0])

	return true
}

// WaitTilOver waits until all channels are closed. RunningStatus channel has multiple senders and one receiver
// Closing a channel on a receiver violates the general channel close principle.
// However, when waitTilOver terminates, it seems safe to close the channel here because all tasks are finished.
func (dag *Dag) WaitTilOver(ctx context.Context) bool {

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

// TODO context cancel 관련 해서 추가 해줘야 하고 start() 같은 경우도 처리 해줘야 한다.
// 약간 이상한 모양인 데이걸 처리하는
// TODO 나중에 수정해주자.

func printRunningStatus(status *printStatus) {
	if status == nil {
		return
	}
	fmt.Printf("nodeId:%s, status:%d\n", status.nodeId, status.rStatus)

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

func (dag *Dag) SetTimeout(d time.Duration) {
	if dag.bTimeout == false {
		dag.bTimeout = true
	}
	dag.Timeout = d
}

func (dag *Dag) DisableTimeout() {
	if dag.bTimeout == true {
		dag.bTimeout = false
	}
}

// AddCommand add command to node. TODO node 의 field 가 늘어날때 수정해준다.
func (dag *Dag) AddCommand(id, c string, cmd string) (node *Node) {
	node = nil
	if n, b := nodeExist(dag, id); b == true {
		n.commands = cmd
	}
	return
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

func (dag *Dag) AddNodeToStartNode(to *Node) error {

	if to == nil {
		return fmt.Errorf("node is nil")
	}

	fromNode := dag.startNode
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
		fmt.Println("error")
	}

	return nil
}
