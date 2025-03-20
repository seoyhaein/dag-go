package dag_go

import (
	"context"
	"fmt"
	"golang.org/x/sync/errgroup"
	"strconv"
	"strings"
	// (do not erase) goroutine 디버깅용
	"github.com/dlsniper/debugger"
)

// TODO panic 지우기. 중요 에러일 경우는 빠르게 종료 시키자.
// TODO 외부 공개 api 가 내부 api 에 들어가 있는 경우 있음. 질서를 잡자.

type Node struct {
	Id string
	// 컨테이너 빌드를 위한 from 이미지.
	// TODO 이거 지워줘야 한다.
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
	runner         func(ctx context.Context, result chan *printStatus)

	// add by seoy race 문제 해결을 위해 22/09/08
	//nodeStatus chan *printStatus

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
	}

	// errgroup 과 새로운 컨텍스트 생성
	eg, ctx := errgroup.WithContext(ctx)
	i := len(n.parentVertex)
	var try bool = true

	for j := 0; j < i; j++ {
		k := j // closure 캡처 문제 해결
		c := n.parentVertex[k]
		try = eg.TryGo(func() error {
			// 각 고루틴에 디버깅 라벨 설정
			debugger.SetLabels(func() []string {
				return []string{
					"preFlight: nodeId", n.Id,
					"channelIndex", strconv.Itoa(k),
				}
			})
			select {
			case result := <-c:
				if result == Failed {
					return fmt.Errorf("node %s: parent channel returned Failed", n.Id)
				}
				return nil
			case <-ctx.Done():
				// 컨텍스트 취소 시 에러 반환
				return ctx.Err()
			}
		})
		if !try {
			break
		}
	}

	// 모든 고루틴이 종료될 때까지 대기
	err := eg.Wait()
	if err == nil && try {
		n.succeed = true
		// do not erase
		// fmt.Println 은 표준 출력(stdout)으로 나와서 벤치마크테스트 결과에 나와서 benchstat 이 그 출력 결과물을 파싱하는데 에러가 발생하여서 표준 에러 (stderr) 결과를 출력하도록 바꿈.
		Log.Println("Preflight succeeded for node", n.Id)
		return &printStatus{Preflight, n.Id}
	}

	n.succeed = false
	// do not erase
	// fmt.Println 은 표준 출력(stdout)으로 나와서 벤치마크테스트 결과에 나와서 benchstat 이 그 출력 결과물을 파싱하는데 에러가 발생하여서 표준 에러 (stderr) 결과를 출력하도록 바꿈.
	Log.Println("Preflight failed for node", n.Id, "error:", err)
	return &printStatus{PreflightFailed, noNodeId}
}

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

// inFlight preFlight, inFlight, postFlight 에서의 node 는 같은 노드이다.
// runner 에서 순차적으로 동작한다.
func inFlight(n *Node) *printStatus {
	if n == nil {
		panic(fmt.Errorf("node is nil"))
	}

	// 시작 노드와 종료 노드는 실행 없이 succeed 를 true 로 설정
	if n.Id == StartNode || n.Id == EndNode {
		n.succeed = true
	} else if n.succeed { // 기존에 succeed 가 true 일 때만 실행 진행
		// TODO Execute 재정의 하거나 새롭게 구현할때 참고해야 함.
		if _, err := n.Execute(); err != nil {
			n.succeed = false
		}
		// else: n.succeed remains true
	}

	// 결과에 따라 메시지를 출력하고 printStatus 를 반환
	if n.succeed {
		fmt.Println("InFlight", n.Id)
		return &printStatus{InFlight, n.Id}
	}

	fmt.Println("InFlightFailed", n.Id)
	return &printStatus{InFlightFailed, n.Id}
}

// postFlight preFlight, inFlight, postFlight 에서의 node 는 같은 노드이다.
// runner 에서 순차적으로 동작한다.
func postFlight(n *Node) *printStatus {
	if n == nil {
		panic(fmt.Errorf("node is nil"))
		// 안정화 버전에서는 panic 대신 적절한 오류 처리를 할 수 있음
	}

	// 종료 노드(EndNode)인 경우 별도로 처리
	if n.Id == EndNode {
		fmt.Println("FlightEnd", n.Id)
		return &printStatus{FlightEnd, n.Id}
	}

	// n.succeed 값에 따라 보낼 결과를 결정
	var result runningStatus
	if n.succeed {
		result = Succeed
	} else {
		result = Failed
	}

	// 모든 자식 채널에 result 값을 보내고, 채널을 닫음
	for _, c := range n.childrenVertex {
		c <- result
		close(c)
	}

	fmt.Println("PostFlight", n.Id)
	return &printStatus{PostFlight, n.Id}
}

func createNode(id string, r Runnable) *Node {
	return &Node{
		Id:         id,
		RunCommand: r,
	}
}

func createNodeWithId(id string) *Node {
	return &Node{
		Id: id,
	}
}

// Execute 이것을 작성하면 된다.
// TODO 7 은 FlightEnd 인지 확인하자. 상수들에 대해서 정리하자.
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
