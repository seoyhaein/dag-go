package dag_go

import (
	"github.com/sirupsen/logrus"
)

// https://github.com/sirupsen/logrus
// https://blog.advenoh.pe.kr/go/Go%EC%97%90%EC%84%9C%EC%9D%98-%EB%A1%9C%EA%B7%B8%EA%B9%85-Logging-in-Go/
var log = logrus.New()

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
