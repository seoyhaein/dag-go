package main

import (
	"context"
	"fmt"
	"time"

	dag_go "github.com/seoyhaein/dag-go"
	"github.com/seoyhaein/dag-go/debugonly"
)

// TODO 이건 별도로 테스트 서버에서 해야함. 내 개발 노트북에서 하면 뻗음.

// HeavyCommand simulates a CPU-and-IO-intensive workload for DAG load testing.
type HeavyCommand struct {
	Iterations int           // 반복 연산 횟수
	Sleep      time.Duration // 부하 시뮬레이션용 sleep 시간
}

// RunE executes a CPU-bound loop followed by a context-aware sleep to simulate
// network / disk I/O latency.  Cancellation via ctx is respected during sleep.
func (c *HeavyCommand) RunE(ctx context.Context, _ interface{}) error {
	// CPU load simulation.
	sum := 0
	for i := 0; i < c.Iterations; i++ { //nolint:intrange
		sum += i*i + i%3
	}
	_ = sum // prevent compiler optimisation

	// I/O latency simulation — honour context cancellation.
	select {
	case <-time.After(c.Sleep):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func main() {
	RunHeavyDag()
}

// RunHeavyDag constructs and executes a multi-node DAG with HeavyCommand runners for load testing.
func RunHeavyDag() {
	dag, err := dag_go.InitDag()
	if err != nil {
		panic(fmt.Sprintf("InitDag failed: %v", err))
	}

	/*	dag.SetContainerCmd(&HeavyCommand{
		Iterations: 10_000_000,             // 꽤 많은 연산
		Sleep:      200 * time.Millisecond, // 네트워크/디스크 지연 시뮬레이션
	})*/

	dag.SetContainerCmd(&HeavyCommand{
		Iterations: 10,                   // 꽤 많은 연산
		Sleep:      2 * time.Millisecond, // 네트워크/디스크 지연 시뮬레이션
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
	fmt.Printf("debugger tag: %v\n", debugonly.Enabled())
	fmt.Printf("Heavy DAG run complete. Progress: %.2f\n", dag.Progress())
}
