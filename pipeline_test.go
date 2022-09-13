package dag_go

import (
	"context"
	"testing"
)

func TestNewPipeline(t *testing.T) {
	d := NewDag()
	d.AddEdge(d.StartNode.Id, "1")
	d.AddEdge("1", "2")
	d.AddEdge("1", "3")
	d.AddEdge("1", "4")
	d.AddEdge("2", "5")
	d.AddEdge("5", "6")

	/*d.AddCommand("1", `sleep 1`)
	d.AddCommand("2", `sleep 1`)
	d.AddCommand("3", `sleep 1`)
	d.AddCommand("4", `sleep 1`)
	d.AddCommand("5", `sleep 1`)
	d.AddCommand("6", `sleep 1`)*/

	err := d.FinishDag()
	if err != nil {
		panic(err)
	}
	//TODO 일단 구현에서 빠진 부분을 일단 보완하고 추가적으로 진행한다.
	copied := CopyDag(d, "78")
	ctx := context.Background()
	copied.ConnectRunner()
	//copy.DagSetFunc()
	copied.GetReady(ctx)
	copied.Start()
	//time.Sleep(time.Second * 10)
	copied.Wait(ctx)

	//copy := CopyDag(d)
	/*ctx := context.Background()
	d.DagSetFunc()
	d.GetReady(ctx)
	d.Start()
	d.Wait(ctx)*/
}

// 오류있어서 일단 테스트 해봐야 함.
func TestNewPipeline01(t *testing.T) {
	d := NewDag()
	d.AddEdge(d.StartNode.Id, "1")
	d.AddEdge("1", "2")
	d.AddEdge("1", "3")
	d.AddEdge("1", "4")
	d.AddEdge("2", "5")
	d.AddEdge("5", "6")

	/*d.AddCommand("1", `sleep 1`)
	d.AddCommand("2", `sleep 1`)
	d.AddCommand("3", `sleep 1`)
	d.AddCommand("4", `sleep 1`)
	d.AddCommand("5", `sleep 1`)
	d.AddCommand("6", `sleep 1`)*/

	err := d.FinishDag()
	if err != nil {
		panic(err)
	}
	//TODO 일단 구현에서 빠진 부분을 일단 보완하고 추가적으로 진행한다.

	pipe := NewPipeline()
	pipe.NewDags(1000, d)
	pipe.Start(context.Background())

	/*copy := CopyDag(d, "78")
	ctx := context.Background()
	copy.DagSetFunc()
	copy.GetReady(ctx)
	copy.Start()
	//time.Sleep(time.Second * 10)
	copy.Wait(ctx)*/

	//copy := CopyDag(d)
	/*ctx := context.Background()
	d.DagSetFunc()
	d.GetReady(ctx)
	d.Start()
	d.Wait(ctx)*/
}

func BenchmarkStart(b *testing.B) {
	for i := 0; i < b.N; i++ {
		demo(1)
	}
}

func demo(i int) {
	d := NewDag()
	d.AddEdge(d.StartNode.Id, "1")
	d.AddEdge("1", "2")
	d.AddEdge("1", "3")
	d.AddEdge("1", "4")
	d.AddEdge("2", "5")
	d.AddEdge("5", "6")
	err := d.FinishDag()
	if err != nil {
		panic(err)
	}
	pipe := NewPipeline()
	pipe.NewDags(i, d)
	pipe.Start(context.Background())
}
