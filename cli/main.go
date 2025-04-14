package main

import (
	"context"
	"fmt"
	"github.com/seoyhaein/dag-go"
	"time"
)

// TODO 이건 별도로 테스트 서버에서 해야함. 내 개발 노트북에서 하면 뻗음.

type HeavyCommand struct {
	Iterations int           // 반복 연산 횟수
	Sleep      time.Duration // 부하 시뮬레이션용 sleep 시간
}

// TODO node 수정해줘야 함.
func (c *HeavyCommand) RunE(_ interface{}) error {
	// CPU 부하 시뮬레이션
	sum := 0
	for i := 0; i < c.Iterations; i++ {
		sum += i*i + i%3
	}
	_ = sum // 쓰이지 않지만 최적화 방지

	// 네트워크/디스크 I/O 지연 시뮬레이션
	time.Sleep(c.Sleep)

	return nil
}

func main() {
	RunHeavyDag()
}

func RunHeavyDag() {
	dag, err := dag_go.InitDag()
	if err != nil {
		panic(fmt.Sprintf("InitDag failed: %v", err))
	}

	dag.SetContainerCmd(&HeavyCommand{
		Iterations: 10_000_000,             // 꽤 많은 연산
		Sleep:      200 * time.Millisecond, // 네트워크/디스크 지연 시뮬레이션
	})

	// 노드 구성 동일
	nodeIDs := []string{"A", "B1", "B2", "C", "D1", "D2", "E"}
	for _, id := range nodeIDs {
		dag.CreateNode(id)
	}

	edges := []struct{ from, to string }{
		{dag.StartNode.ID, "A"},
		{"A", "B1"}, {"A", "B2"},
		{"B1", "C"}, {"B2", "C"},
		{"C", "D1"}, {"C", "D2"},
		{"D1", "E"}, {"D2", "E"},
	}
	for _, edge := range edges {
		if err := dag.AddEdgeIfNodesExist(edge.from, edge.to); err != nil {
			panic(fmt.Sprintf("failed to add edge %s -> %s: %v", edge.from, edge.to, err))
		}
	}

	if err := dag.FinishDag(); err != nil {
		panic(fmt.Sprintf("FinishDag failed: %v", err))
	}

	dag.ConnectRunner()

	ctx := context.Background()
	if !dag.GetReady(ctx) {
		panic("GetReady failed")
	}
	if !dag.Start() {
		panic("Start failed")
	}

	if !dag.Wait(ctx) {
		panic("DAG execution failed")
	}

	fmt.Printf("Heavy DAG run complete. Progress: %.2f\n", dag.Progress())
}
