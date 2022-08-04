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
func NewDag() *Dag {
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

	node := &Node{Id: id}
	node.parentDag = dag
	dag.nodes[id] = node

	return node
}

func (dag *Dag) createNodeFromXmlNode(xnode *xmlNode) *Node {

	if xnode == nil {
		return nil
	}

	// 중복된 id 가 있으면 nil 리턴
	for _, n := range dag.nodes {
		if n.Id == xnode.id {
			return nil
		}
	}

	node := &Node{Id: xnode.id}
	node.commands = xnode.command
	node.parentDag = dag
	dag.nodes[xnode.id] = node

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

// TODO 향후 xmlNode 와 Node 가 통합되면 아래 메서드들 수정해주어야 함.

func (dag *Dag) AddEdgeFromXmlNode(from, to *xmlNode) error {

	if from == nil {
		return fmt.Errorf("from-xmlNode is nil")
	}

	if to == nil {
		return fmt.Errorf("to-xmlNode is nil")
	}

	if from.id == to.id {
		return fmt.Errorf("from.id and to.id are same")
	}

	fromNode := dag.nodes[from.id]
	if fromNode == nil {
		fromNode = dag.createNodeFromXmlNode(from)

		if fromNode == nil {
			return fmt.Errorf("fromNode: createNodeFromXmlNode return nil")
		}
	}
	toNode := dag.nodes[to.id]
	if toNode == nil {
		toNode = dag.createNodeFromXmlNode(to)

		if toNode == nil {
			return fmt.Errorf("toNode: createNodeFromXmlNode return nil")
		}
	}

	// 원은 허용하지 않는다.
	// TODO 향후 어떻게 고칠지 생각해 보자.
	// 일단은 주석처리 한다.
	/*if strings.Contains(toNode.Id, "start_node") {
		return fmt.Errorf("circle is not allowed.")
	}*/

	fromNode.children = append(fromNode.children, toNode)
	toNode.parent = append(toNode.parent, fromNode)

	edge, check := dag.createEdge(fromNode.Id, toNode.Id)
	if check == Fault || check == Exist {
		return fmt.Errorf("Edge cannot be created")
	}

	if edge != nil {
		fromNode.childrenVertex = append(fromNode.childrenVertex, edge.vertex)
		toNode.parentVertex = append(toNode.parentVertex, edge.vertex)
	} else {
		fmt.Errorf("vertex is nil")
	}
	return nil
}

func (dag *Dag) AddEdgeFromXmlNodeToStartNode(to *xmlNode) error {
	/*fromNode := dag.nodes[from.id]
	if fromNode == nil {
		fromNode = dag.createNodeFromXmlNode(from)
	}*/
	if to == nil {
		return fmt.Errorf("xmlNode is nil")
	}

	fromNode := dag.startNode
	toNode := dag.nodes[to.id]
	if toNode == nil {
		toNode = dag.createNodeFromXmlNode(to)
	}

	if fromNode == toNode {
		return fmt.Errorf("from-node and to-node are same")
	}

	// 원은 허용하지 않는다.
	// TODO 향후 어떻게 고칠지 생각해 보자.
	// 일단은 주석처리 한다.
	/*if strings.Contains(toNode.Id, "start_node") {
		return fmt.Errorf("circle is not allowed.")
	}*/

	fromNode.children = append(fromNode.children, toNode)
	toNode.parent = append(toNode.parent, fromNode)

	//fromNode.outdegree++
	//toNode.indegree++

	//check := dag.createEdge(fromNode.Id, toNode.Id)
	dag.createEdge(fromNode.Id, toNode.Id)
	// 생성 시키면 0, 이 존재하면 1, 에러면 2
	// TODO 향후 에러코드 만들면 수정해야함.
	/*if check == 0 {
		fmt.Println("만들어줌.")
	}
	if check == 1 {
		fmt.Println("존재함")
	}

	if check == 2 {
		fmt.Println("에러")
	}*/

	v := dag.getVertex(fromNode.Id, toNode.Id)

	if v != nil {
		fromNode.childrenVertex = append(fromNode.childrenVertex, v)
		toNode.parentVertex = append(toNode.parentVertex, v)
	} else {
		fmt.Println("error")
	}

	return nil
}

func (dag *Dag) AddEdgeFromXmlNodeToEndNode(from *xmlNode) error {

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

// finishDag finally, connect end_node to dag
func (dag *Dag) finishDag() error {

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

func (dag *Dag) dagSetFunc(ctx context.Context) {

	n := len(dag.nodes)
	if n < 1 {
		return
	}

	for _, v := range dag.nodes {
		setFunc(ctx, v)
	}
}

func (dag *Dag) getReady(ctx context.Context) bool {
	n := len(dag.nodes)
	if n < 1 {
		return false
	}

	for _, v := range dag.nodes {
		go v.runner(ctx, v, dag.RunningStatus)
	}

	return true
}

// start start_node has one vertex. That is, it has only one channel and this channel is not included in the edge.
// It is started by sending a value to this channel when starting the dag's operation.
func (dag *Dag) start() bool {
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

// waitTilOver waits until all channels are closed. RunningStatus channel has multiple senders and one receiver
// Closing a channel on a receiver violates the general channel close principle.
// However, when waitTilOver terminates, it seems safe to close the channel here because all tasks are finished.
func (dag *Dag) waitTilOver(ctx context.Context) bool {

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

// TODO 일단 그냥 대충 이렇게 함 추후 수정할 예정임.

func (dag *Dag) Ready(ctx context.Context) {
	dag.dagSetFunc(ctx)
	dag.getReady(ctx)
}

// time.Second 이거 지워야 함. 그냥 넣어줌.

func (dag *Dag) Start() {
	dag.start()
	dag.waitTilOver(nil)
}

func (dag *Dag) Stop() {
	time.After(time.Second * 2)
}

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
