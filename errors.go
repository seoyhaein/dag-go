package dag_go

type (
	ErrorType int

	systemError struct {
		errorType ErrorType
		reason    error
	}
)

const (
	AddEdge ErrorType = iota
	AddEdgeFromXmlNode
	AddEdgeFromXmlNodeToStartNode
	AddEdgeFromXmlNodeToEndNode
	addEndNode
	finishDag
)
