package podbridge

import (
	"github.com/sirupsen/logrus"
)

var Log = logrus.New()

type ContainerStatus int

const (
	Created   ContainerStatus = iota //0
	Running                          // 1
	Exited                           // 2
	ExitedErr                        // 3
	Healthy                          // 4
	Unhealthy                        // 5
	Dead                             // 6
	Paused                           // 7
	UnKnown                          // 8
	None                             // 9
)

// channel buffer size
const (
	Min int = 1
	Max int = 100
)
