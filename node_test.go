package dag_go

import (
	"fmt"
	"testing"
)

func TestCloneGraph(t *testing.T) {

	var copied map[string]*Node
	dag := NewDag()
	dag.AddEdge(dag.StartNode.Id, "1")
	dag.AddEdge("1", "2")
	dag.AddEdge("1", "3")
	dag.AddEdge("1", "4")
	dag.AddEdge("2", "5")
	dag.AddEdge("5", "6")
	dag.AddCommand("1", "")
	dag.FinishDag()

	copied, _ = cloneGraph(dag.nodes)

	n := len(copied)
	n1 := len(dag.nodes)
	fmt.Println("8이 나와야 정상", n1)
	fmt.Println("8이 나와야 정상", n)

}
