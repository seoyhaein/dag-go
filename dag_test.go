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
	// 2번에 실패를 두고 테스트 진행하자.
	dag.AddEdge(dag.StartNode.Id, "1")
	dag.AddEdge("1", "2")
	dag.AddEdge("1", "3")
	dag.AddEdge("1", "4")
	dag.AddEdge("2", "5")
	dag.AddEdge("5", "6")
	// TODO 수정해야함.
	dag.AddCommand("1", "")

	err := dag.FinishDag()
	if err != nil {
		t.Errorf("%+v", err)
	}
	ctx := context.Background()
	dag.DagSetFunc()
	dag.GetReady(ctx)
	b1 := dag.Start()
	assert.Equal(true, b1, "true")

	// 에러 발생하게 했다.
	b2 := dag.Wait(ctx)
	assert.Equal(true, b2, "true")

}

func TestCopyDag1(t *testing.T) {

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
	copy := CopyDag(d, "78")
	ctx := context.Background()
	copy.DagSetFunc()
	copy.GetReady(ctx)
	copy.Start()
	//time.Sleep(time.Second * 10)
	copy.Wait(ctx)

	//copy := CopyDag(d)
	/*ctx := context.Background()
	d.DagSetFunc()
	d.GetReady(ctx)
	d.Start()
	d.Wait(ctx)*/
}
