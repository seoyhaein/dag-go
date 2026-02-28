package dag_go

import "errors"

// ErrCycleDetected is returned by FinishDag when the graph contains a directed cycle.
// Callers can test for this condition with errors.Is(err, ErrCycleDetected).
var ErrCycleDetected = errors.New("cycle detected in DAG")

// ErrorType identifies the DAG operation that produced a systemError.
type (
	ErrorType int

	systemError struct {
		errorType ErrorType
		reason    error
	}
)

// AddEdge, StartDag, AddEdgeIfNodesExist, addEndNode, FinishDag are the
// ErrorType values that identify which DAG operation recorded an error.
const (
	AddEdge ErrorType = iota
	StartDag
	AddEdgeIfNodesExist
	addEndNode
	FinishDag
)
