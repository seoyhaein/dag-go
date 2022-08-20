package dag_go

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// https://github.com/stretchr/testify
// https://pkg.go.dev/github.com/google/uuid#IsInvalidLengthError
func TestNewDag(t *testing.T) {
	assert := assert.New(t)

	dag := NewDag()
	if assert.NotNil(dag) {
		err := dag.FinishDag()

		if err != nil {
			t.Errorf("%+v", err)
		}
		ctx := context.Background()
		dag.DagSetFunc(ctx)
		dag.GetReady(ctx)

		v := dag.getVertex(dag.startNode.Id, dag.endNode.Id)
		assert.NotNil(v)

		numOfEdges := len(dag.Edges)
		assert.Equal(1, numOfEdges, "There should be only one channel.")

		b1 := dag.Start()
		assert.Equal(true, b1, "true")
		b2 := dag.WaitTilOver(ctx)
		assert.Equal(true, b2, "true")

		// test for start_node, end_node
	}
}

func TestSimpleDag(t *testing.T) {
	assert := assert.New(t)
	dag := NewDag()

	// create dag
	dag.AddEdge(dag.startNode.Id, "1")
	dag.AddEdge("1", "2")
	dag.AddEdge("2", "3")
	dag.AddEdge("3", "4")
	// TODO 수정해야함.
	dag.AddCommand("1", "", "")

	err := dag.FinishDag()
	if err != nil {
		t.Errorf("%+v", err)
	}
	ctx := context.Background()
	dag.DagSetFunc(ctx)
	dag.GetReady(ctx)
	b1 := dag.Start()
	assert.Equal(true, b1, "true")

	// 에러 발생하게 했다.
	b2 := dag.WaitTilOver(ctx)
	assert.Equal(true, b2, "true")

}

// pod는 아직 연결하지 않는다.
// 컨테이너 실패시 dag 가 정상적으로 멈추는지 확인해야한다.
// 기타 부가적인 api 개발한다.

func TestPodbridge01(t *testing.T) {
	assert := assert.New(t)
	dag := NewDag()

	// create dag
	dag.AddEdge(dag.startNode.Id, "1")
	dag.AddEdge("1", "2")
	dag.AddEdge("2", "3")
	dag.AddEdge("3", "4")

	// TODO 수정해야 함.
	dag.AddCommand("1", "", "")

	err := dag.FinishDag()
	if err != nil {
		t.Errorf("%+v", err)
	}
	ctx := context.Background()
	dag.DagSetFunc(ctx)
	dag.GetReady(ctx)
	b1 := dag.Start()
	assert.Equal(true, b1, "true")

	// 에러 발생하게 했다.
	b2 := dag.WaitTilOver(ctx)
	assert.Equal(true, b2, "true")

}

// TODO
// channel 제대로 닫혔는지 확인해야 한다.
// https://go101.org/article/channel-closing.html
