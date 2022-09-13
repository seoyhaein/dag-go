package dag_go

import (
	"github.com/sirupsen/logrus"
)

var Log = logrus.New()

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
	Start runningStatus = iota
	Preflight
	PreflightFailed
	InFlight
	InFlightFailed
	PostFlight
	PostFlightFailed
	FlightEnd
	Failed
	Succeed
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

// channel buffer size
const (
	Max           int = 100
	Min           int = 1
	StatusDefault int = 3
)
