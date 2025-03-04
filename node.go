package dag_go

import (
	"context"
	"fmt"
	"golang.org/x/sync/errgroup"
	"strings"
	// (do not erase) goroutine 디버깅용
	//"github.com/dlsniper/debugger"
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

	// add by seoy race 문제 해결을 위해 22/09/08
	//nodeStatus chan *printStatus

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

// preFlight preFlight, inFlight, postFlight 에서의 node 는 같은 노드이다.
// runner 에서 순차적으로 동작한다.
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

// inFlight preFlight, inFlight, postFlight 에서의 node 는 같은 노드이다.
// runner 에서 순차적으로 동작한다.
func inFlight(n *Node) *printStatus {
	if n == nil {
		panic(fmt.Errorf("node is nil"))
		// (do not erase) 안정화 버전 나올때는 panic 을 리턴으로 처리
		//return &printStatus{InFlightFailed, noNodeId}
	}
	// (do not erase) goroutine 디버깅용
	/*debugger.SetLabels(func() []string {
		return []string{
			"inFlight: nodeId", n.Id,
		}
	})*/

	if n.Id == StartNode || n.Id == EndNode {
		n.succeed = true
	} else {
		// 성골할때만 명령을 실행시키고, 실패할경우는 채널에 값만 흘려 보낸다.
		// TODO 리턴 코드 작성하자.
		if n.succeed {
			_, err := n.Execute()
			if err != nil {
				n.succeed = false
			} else {
				n.succeed = true
			}
		}
	}
	if n.succeed {
		fmt.Println("InFlight", n.Id)
		return &printStatus{InFlight, n.Id}
	} else {
		fmt.Println("InFlightFailed", n.Id)
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
	// (do not erase) goroutine 디버깅용
	/*debugger.SetLabels(func() []string {
		return []string{
			"postFlight: nodeId", n.Id,
		}
	})*/
	if n.Id == EndNode {
		fmt.Println("FlightEnd", n.Id)
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
	fmt.Println("PostFlight", n.Id)
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

// getNextNode The first node to enter is fetched and the corresponding node is deleted.
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
