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
