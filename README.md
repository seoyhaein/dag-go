# dag-go
[![Go Reference](https://pkg.go.dev/badge/github.com/seoyhaein/dag-go.svg)](https://pkg.go.dev/github.com/seoyhaein/dag-go)
[![Build Status](https://app.travis-ci.com/seoyhaein/dag-go.svg?branch=main)](https://app.travis-ci.com/seoyhaein/dag-go)
[![CodeFactor](https://www.codefactor.io/repository/github/seoyhaein/dag-go/badge/main)](https://www.codefactor.io/repository/github/seoyhaein/dag-go/overview/main)

## TODO
- TODO 다 지우기.
~~- dag.go 에서 에러 리턴하는 방법 좀더 연구하자. 현재 error 를 리턴하지만 외부로 빼지는 않는다. (중요, 우선순위는 낮음.)~~
- id string 체크 해줘야 함.
- travis 안되서 github action 으로 바꾸자.  
~~- podbridge5 와 분리하자.~~  
~~- 테스트 과정에서 디렉토리 더러워지는 현상 개선 및 자동화 방안 생각하지.~~
- 고루틴 채널 사용시에 블럭이 발생할 수 있는 개연성을 생각해서 time-out 을 넣어서 cancel 해주는 것을 한번 생각해보자. 아직 이건 구현 안되어 있음. (중요)  
- 여러 테스트 및 프로파일링도 진행 필요.  
~~- 필요없는 데이터는 일단 정리를 하자.~~
~~- addvetex, createnoe 분리.~~
- 고루틴 메모리 릭 검사해줘야 함. (중요, 우선순위 낮음.)
~~- 버그는 수정했지만, 계속 수정해줘야 함.~~ 
- lint 수정 중