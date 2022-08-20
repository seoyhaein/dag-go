package dag_go

import (
	"context"
	"fmt"
	"strings"

	"github.com/seoyhaein/utils/shellexecmd"
	"golang.org/x/sync/errgroup"
)

type Node struct {
	Id string

	children  []*Node // children
	parent    []*Node // parents
	parentDag *Dag    // 자신이 소속되어 있는 Dag

	commands       string
	status         string
	childrenVertex []chan int
	parentVertex   []chan int

	runner func(ctx context.Context, n *Node, result chan *printStatus)

	// for xml parsing
	from []string
	to   []string

	// TODO re-thinking
	// https://yoongrammer.tistory.com/36
	context.Context

	// TODO commands 복수로 할지 하나로 할지는 고민한다.
	cmd []*Command
	c   *Command

	// 추후 commands string 과 교체
	bashCommand []string
	// 컨테이너 빌드를 위한 from 이미지.
	ImageName string
}

// (do not erase) close 해주는 것 : func (dag *Dag) waitTilOver(ctx context.Context) bool  에서 defer close(dag.RunningStatus) 해줌
// (do not erase) 너무 중요.@@@@ 채널 close 방식 확인하자. https://go101.org/article/channel-closing.html 너무 좋은 자료. 왜 제목을 101 이라고 했지 중급이상인데.

func setFunc(ctx context.Context, n *Node) {
	n.runner = func(ctx context.Context, n *Node, result chan *printStatus) {
		//defer close(result)
		r := preFlight(ctx, n)

		result <- r
		//TODO 특정 노드가 실패하면 여기서 빠져 나가야 할 것 같다.
		r = inFlight(ctx, n)
		result <- r
		//TODO 특정 노드가 실패하면 여기서 빠져 나가야 할 것 같다.
		r = postFlight(ctx, n)
		result <- r
		//TODO 특정 노드가 실패하면 여기서 빠져 나가야 할 것 같다.
	}
}

func preFlight(ctx context.Context, n *Node) *printStatus {

	if n == nil {
		return &printStatus{PreflightFailed, noNodeId}
	}
	// 성공하면 context 사용한다.
	eg, _ := errgroup.WithContext(ctx)
	i := len(n.parentVertex) // 부모 채널의 수
	for j := 0; j < i; j++ {
		// (do not erase) 중요!! 여기서 들어갈 변수를 세팅않해주면 에러남.
		j := j
		c := n.parentVertex[j]
		eg.Go(func() error {
			<-c
			return nil
		})
	}
	if err := eg.Wait(); err == nil { // 대기
		return &printStatus{Preflight, n.Id}
	}
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

// TODO 특정 노드에서 실행 취소가 발생할 수 있도록 해야 한다.
func inFlight(ctx context.Context, n *Node) *printStatus {

	if n == nil {
		return &printStatus{InFlightFailed, noNodeId}
	}

	if n.Id == StartNode {
		fmt.Println("start dag-go ", n.Id)
	}

	if n.Id == EndNode {
		fmt.Println("end all tasks", n.Id)
	}

	var bResult = false

	if n.Id == StartNode || n.Id == EndNode {
		bResult = true
	} else { // TODO debug 모드때문에 넣어 놓았음. AddEdge 하면 commands. 안들어감. 추후 삭제하거나, 다른 방향으로 작성해야함.
		if len(strings.TrimSpace(n.commands)) == 0 {
			fmt.Println(n.Id)
			// TODO command 추가
			if n.c != nil {
				n.c.Execute()
			}
			bResult = true
		} else {
			bResult = shellexecmd.Runner(n.commands)
		}
	}

	if bResult {
		return &printStatus{InFlight, n.Id}
	} else {
		return &printStatus{InFlightFailed, n.Id}
	}
}

func postFlight(ctx context.Context, n *Node) *printStatus {

	if n == nil {
		return &printStatus{PostFlightFailed, noNodeId}
	}

	k := len(n.childrenVertex)
	for j := 0; j < k; j++ {
		c := n.childrenVertex[j]
		c <- 1
		close(c)
	}

	if n.Id == EndNode {
		return &printStatus{FlightEnd, n.Id}
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

// create command, 일단 간단히 기능 구현만 한다. 추후 보강한다.

/*func InitCommand() (cmd *Command) {
	cmd = &Command{
		RunE: func() error {
			fmt.Println("hello world")
			return nil
		},
	}

	return
}*/

/*func (n *Node) AddCommand(cmds ...*Command) {

	for _, cmd := range cmds {
		n.cmd = append(n.cmd, cmd)
	}
}*/

// 컨테이너 생성하고 하는 루틴이 들어가야 할듯하다.
// 일단은 직넙 podbrdige 와 직접 연결하고 추후에는 함수를 만들어서 간접적으로 연결 할 수 있도록 한다. 즉, 다른 프로젝트에서 해당 함수를 이용해서 연결 하는 방향으로 간다.

/*func (n *Node) AddCommandT() {

	var cmd = &Command{
		RunE: func() error {
			fmt.Println("hello world")
			return nil
		},
	}

	n.c = cmd
}*/

// TODO 내일 수정하자. func CreateCommand(n *Node, r Runnable) *Command -> command.go 참고
// TODO node 에 명령어 등 넣고 생성하는 메서드
// (dag *Dag) createNode(id string) *Node  에 들어가는데 상위 노출 되는 시점을 찾아서 외부 노출 방안을 고민해보자.
func createNode(id string) (node *Node) {
	node = &Node{
		Id: id,
		c:  new(Command),
	}
	node.c.this = node
	return
}

func CreateNode(id string, r Runnable) *Node {

	createNode(id)
	return nil
}
