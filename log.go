package dag_go

import (
	"github.com/sirupsen/logrus" //nolint:depguard // this is used by logutils.Log for logging
)

// Log is the package-level logger used throughout dag-go.
var Log = logrus.New()
