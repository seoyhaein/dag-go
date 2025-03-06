# dag-go
[![Go Reference](https://pkg.go.dev/badge/github.com/seoyhaein/dag-go.svg)](https://pkg.go.dev/github.com/seoyhaein/dag-go)
[![Build Status](https://app.travis-ci.com/seoyhaein/dag-go.svg?branch=main)](https://app.travis-ci.com/seoyhaein/dag-go)
[![CodeFactor](https://www.codefactor.io/repository/github/seoyhaein/dag-go/badge/main)](https://www.codefactor.io/repository/github/seoyhaein/dag-go/overview/main)

## TODO
- TODO 다 지우기.
- dag.go 에서 에러 리턴하는 방법 좀더 연구하자. 현재 error 를 리턴하지만 외부로 빼지는 않는다.
- 아래와 같은 방식으로, 이걸 좀더 연구해보자.
```go
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


```
- id string 체크 해줘야 함.
- node, dag, edge, vertex 개념을 정리하자. edge 는 현재 dag 에 정의 되어 있다. 그리고, vertex 는 edge 에 있다.
- travis 안되서 github action 으로 바꾸자.  
- podbridge5 와 분리하자.  