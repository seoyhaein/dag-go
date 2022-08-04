package dag_go

type (
	runningStatus int

	printStatus struct {
		rStatus runningStatus
		nodeId  string
	}
)

// createEdgeErrorType 0 if created, 1 if exists, 2 if error.
type createEdgeErrorType int

const (
	Create createEdgeErrorType = iota
	Exist
	Fault
)

// The status displayed when running the runner on each node.
const (
	Preflight runningStatus = iota
	PreflightFailed
	InFlight
	InFlightFailed
	PostFlight
	PostFlightFailed
	FlightEnd
)

const (
	nodes   = "nodes"
	node    = "node"
	id      = "id"
	from    = "from"
	to      = "to"
	command = "command"
)

const (
	StartNode = "start_node"
	EndNode   = "end_node"
)

//It is the node ID when the condition that the node cannot be created.
const noNodeId = "-1"
