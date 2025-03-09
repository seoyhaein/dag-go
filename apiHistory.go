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
