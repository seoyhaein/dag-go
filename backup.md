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
```