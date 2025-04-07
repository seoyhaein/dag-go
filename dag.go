package dag_go

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/seoyhaein/utils"
	"math/rand"
	"strings"
	"time"
)

// rand 의 경우,
// rand/v2 가 있음. golang.org/x/exp/rand
// Go 팀의 실험적 확장 패키지로, 보다 다양한 PRNG 알고리즘(예: PCG, XorShift 등)과 추가 기능을 제공합
// 성능이나 난수 품질 면에서 더 나은 옵션을 제공할 수 있지만, 아직 실험적이므로 API 가 변경될 수 있음
// 새로운 기능을 활용하고 싶거나, 특정 알고리즘이 필요한 경우 고려할 수 있음
// 안정성과 호환성이 중요하다면 math/rand 를 사용하는 것이 좋고, 추가 기능이나 개선된 성능이 필요하다면 rand/v2(또는 golang.org/x/exp/rand)를 검토할 수 있음

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
	// TODO 생각해보기 wait 및 몇가지 부가적인 메서드들에 대해서. (중요) 필요한가 싶다. 일단 조심스럽게 접근하자.
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
func NewDag() *Dag {
	dag := &Dag{
		nodes:         make(map[string]*Node),
		Id:            uuid.NewString(),
		RunningStatus: make(chan *printStatus, Max),
	}
	return dag
}

func (dag *Dag) StartDag() (*Dag, error) {
	if dag.StartNode = dag.CreateNode(StartNode); dag.StartNode == nil {
		return nil, fmt.Errorf("failed to create start node")
	}
	// 시작 노드에 필수 채널 추가 (parentVertex 채널 삽입)
	dag.StartNode.parentVertex = append(dag.StartNode.parentVertex, make(chan runningStatus, Min))
	return dag, nil
}

func InitDag() (*Dag, error) {
	dag := NewDag()
	if dag == nil {
		return nil, fmt.Errorf("failed to run NewDag")
	}
	return dag.StartDag()
}

// TODO 삭제할 예정, 각노드에서 인터페이스를 받아서 미리 만들어 주는 형식으로 진행할 예정.
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

// CreateNode creates a pointer to a new node, but returns nil if dag has a duplicate node id.
func (dag *Dag) CreateNode(id string) *Node {
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
	// TODO 이걸 빼서 메서드를 만들지 고민하자.
	// 에러 로그를 기록하고 반환하는 클로저 함수
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
		node := dag.CreateNode(id)
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

// AddEdgeIfNodesExist 이 메서드로 업데이트 예정. 노드가 없다면 에러를 리턴한다.
func (dag *Dag) AddEdgeIfNodesExist(from, to string) error {
	// 에러 로그를 기록하고 반환하는 클로저 함수
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

	// 노드를 가져오는 클로저 함수: 노드가 없으면 에러 리턴
	getNode := func(id string) (*Node, error) {
		if node := dag.nodes[id]; node != nil {
			return node, nil
		}
		return nil, logErr(fmt.Errorf("%s: node does not exist", id))
	}

	fromNode, err := getNode(from)
	if err != nil {
		return err
	}
	toNode, err := getNode(to)
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
	dag.EndNode = dag.CreateNode(EndNode)
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

// detectCycleDFS 는 DFS 를 통해 사이클이 있는지 탐지
// visited: 이미 방문한 노드를 기록
// recStack: 현재 재귀 호출 경로(스택)에 있는 노드들을 추적
// true 이면 cycle
func detectCycleDFS(node *Node, visited, recStack map[string]bool) bool {
	if recStack[node.Id] {
		return true
	}
	if visited[node.Id] {
		return false
	}
	visited[node.Id] = true
	recStack[node.Id] = true

	for _, child := range node.children {
		if detectCycleDFS(child, visited, recStack) {
			return true
		}
	}

	recStack[node.Id] = false
	return false
}

// DetectCycle 는 주어진 DAG 에 사이클이 존재하는지 검사
// copyDag 를 사용하여 원본 DAG 의 최소 정보(노드의 ID와 부모/자식 관계)만 복사한 뒤, DFS 로 사이클을 탐지
// true 이면 cycle
func DetectCycle(dag *Dag) bool {
	// copyDag 는 최소 정보만 복사하므로, 사이클 검사에 적합
	newNodes, _ := copyDag(dag)
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	// 새로 복사된 노드들을 대상으로 DFS 를 수행
	for _, node := range newNodes {
		if !visited[node.Id] {
			if detectCycleDFS(node, visited, recStack) {
				return true
			}
		}
	}
	return false
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

func connectRunner(n *Node) {
	n.runner = func(ctx context.Context, result chan *printStatus) {
		defer close(result)
		result <- preFlight(ctx, n)
		result <- inFlight(n)
		result <- postFlight(n)
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
		go v.runner(ctx, ch)
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
			go v.runner(ctx, start)
		}

		if v.Id != StartNode && v.Id != EndNode {
			ch := make(chan *printStatus, StatusDefault)
			chs = append(chs, ch)
			go v.runner(ctx, ch)
		}

		if v.Id == EndNode {
			end = v
		}
	}

	if end == nil {
		panic("EndNode is nil")
	}
	chs = append(chs, endCh)
	go end.runner(ctx, endCh)

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

// copyDag dag 를 복사함. 필요한것만 복사함.
func copyDag(original *Dag) (map[string]*Node, []*Edge) {
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

// CopyDag dag 를 복사함.
func CopyDag(original *Dag, newId string) *Dag {
	// 원본이 nil 이면 nil 반환
	if original == nil {
		return nil
	}
	// newId가 빈 문자열이면 nil 반환
	if utils.IsEmptyString(newId) {
		return nil
	}

	// 새 DAG 인스턴스 생성 및 기본 필드 복사
	copied := &Dag{
		Id:            newId,
		Timeout:       original.Timeout,
		bTimeout:      original.bTimeout,
		ContainerCmd:  original.ContainerCmd,
		validated:     original.validated,
		RunningStatus: make(chan *printStatus, Max),
	}

	// 원본의 Pid 가 비어있지 않다면 복사
	if !utils.IsEmptyString(original.Pid) {
		copied.Pid = original.Pid
	}

	// 노드와 간선 복사는 copyDag 함수로 수행 (shallow copy 의도 유지)
	ns, edges := copyDag(original)
	copied.nodes = ns
	copied.Edges = edges

	// 복사된 노드들의 parentDag 필드를 새 DAG 로 재설정하고,
	// StartNode, EndNode 를 찾아서 할당
	for _, n := range ns {
		n.parentDag = copied
		if n.Id == StartNode {
			copied.StartNode = n
		}
		if n.Id == EndNode {
			copied.EndNode = n
		}
	}

	return copied
}

func CopyEdge(original []*Edge) []*Edge {
	if len(original) == 0 {
		return nil
	}
	copied := make([]*Edge, len(original))
	for i, orig := range original {
		copied[i] = &Edge{
			parentId: orig.parentId,
			childId:  orig.childId,
			vertex:   make(chan runningStatus, Min),
		}
	}
	return copied
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

func edgeExists(edges []*Edge, parentId, childId string) bool {
	for _, edge := range edges {
		if edge.parentId == parentId && edge.childId == childId {
			return true
		}
	}
	return false
}

// findEdgeFromParentId 는 주어진 Edge 슬라이스에서
// parentId가 전달된 id와 일치하는 모든 Edge 들을 필터링하여 반환함
func findEdgeFromParentId(es []*Edge, id string) []*Edge {
	var filteredEdges []*Edge
	for _, edge := range es {
		if edge.parentId == id {
			filteredEdges = append(filteredEdges, edge)
		}
	}
	return filteredEdges
}

// findEdgeFromChildId는 주어진 Edge 슬라이스에서
// childId가 전달된 id와 일치하는 모든 Edge 들을 필터링하여 반환함
func findEdgeFromChildId(es []*Edge, id string) []*Edge {
	var filteredEdges []*Edge // 결과를 저장할 슬라이스
	for _, edge := range es {
		// edge 의 childId가 입력된 id와 일치하면 filteredEdges 에 추가
		if edge.childId == id {
			filteredEdges = append(filteredEdges, edge)
		}
	}
	return filteredEdges
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

// for test

// generateDAG 는 numNodes 개의 노드를 생성하고,
// i < j 인 경우 확률 edgeProb 로 부모-자식 간선을 추가하여 DAG 를 구성
func generateDAG(numNodes int, edgeProb float64) *Dag {
	// 새로운 DAG 생성 (NewDag 는 초기화된 Dag 를 반환한다고 가정)
	dag := NewDag()
	dag.nodes = make(map[string]*Node, numNodes)

	// 노드 생성: "0", "1", ..., "numNodes-1" 의 ID 사용
	for i := 0; i < numNodes; i++ {
		id := fmt.Sprintf("%d", i)
		node := &Node{Id: id}
		dag.nodes[id] = node
	}

	// 간선 생성: i < j 인 경우, edgeProb 확률로 A -> B 간선 추가
	for i := 0; i < numNodes; i++ {
		for j := i + 1; j < numNodes; j++ {
			if rand.Float64() < edgeProb {
				parentID := fmt.Sprintf("%d", i)
				childID := fmt.Sprintf("%d", j)
				edge := &Edge{parentId: parentID, childId: childID}
				dag.Edges = append(dag.Edges, edge)
				// 부모 노드의 자식 리스트에 추가
				dag.nodes[parentID].children = append(dag.nodes[parentID].children, dag.nodes[childID])
				// 자식 노드의 부모 리스트에 추가
				dag.nodes[childID].parent = append(dag.nodes[childID].parent, dag.nodes[parentID])
			}
		}
	}
	return dag
}

// verifyCopiedDag 는 원본 DAG 와 복사된 노드 맵 및 간선 슬라이스가 동일한 구조를 갖는지 검증
// 문제가 있으면 에러 메시지를 모아 하나의 error 로 반환하고, 문제가 없으면 nil 을 반환
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
			errs = append(errs, fmt.Sprintf("[%s] node with id %s missing in newNodes", methodName, id))
			continue
		}
		// ID 비교
		if newNode.Id != origNode.Id {
			errs = append(errs, fmt.Sprintf("[%s] expected node id %s, got %s", methodName, origNode.Id, newNode.Id))
		}
		// 부모 관계 검증
		if len(newNode.parent) != len(origNode.parent) {
			errs = append(errs, fmt.Sprintf("[%s] node %s: expected %d parents, got %d", methodName, id, len(origNode.parent), len(newNode.parent)))
		} else {
			for i, p := range newNode.parent {
				if p.Id != origNode.parent[i].Id {
					errs = append(errs, fmt.Sprintf("[%s] node %s: expected parent %s, got %s", methodName, id, origNode.parent[i].Id, p.Id))
				}
			}
		}
		// 자식 관계 검증
		if len(newNode.children) != len(origNode.children) {
			errs = append(errs, fmt.Sprintf("[%s] node %s: expected %d children, got %d", methodName, id, len(origNode.children), len(newNode.children)))
		} else {
			for i, c := range newNode.children {
				if c.Id != origNode.children[i].Id {
					errs = append(errs, fmt.Sprintf("[%s] node %s: expected child %s, got %s", methodName, id, origNode.children[i].Id, c.Id))
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
		if newEdge.parentId != origEdge.parentId || newEdge.childId != origEdge.childId {
			errs = append(errs, fmt.Sprintf("[%s] edge %d: expected parent %s->child %s, got parent %s->child %s",
				methodName, i, origEdge.parentId, origEdge.childId, newEdge.parentId, newEdge.childId))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf(strings.Join(errs, "\n"))
	}
	return nil
}
