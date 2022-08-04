## 시작하기
먼저, dag-go 는 [dag 자료구조를(방향 비순환 그래프, directed acyclic graph)](https://url.kr/lnq18h) 활용하여 명령 구문을 파이프라인으로 구성하는 것을 목표로 하고 있다.

여기서, 명령 구문을 파이프라인으로 구성한다는 의미는 특정 명령 구문(들) A 와 B, C 가 있다면, A 가 실행이 완료되면 그 A 의 수행 결과물을 가지고 B 가 실행되고 C 는 B 에서 생성된 결과를 가지고 최종적으로 실행 결과를 내는 형태이다.

이러한 A,B,C 구성 형태는 방향성을 가진 비순환 그래프와 비슷하다.

## 전제조건(추가 예정)
dag 자료구조를 이용하여  명령 구문을 파이프라인으로 구성(이하 파이프라인으로 통칭)하려면, 몇가지 개발시 필요한 전제조건들이 있다.

###
1. 파이프라인을 구성할때 시작과 끝 노드를 자동으로 생성해주어야 한다. 시작노드를(startNode) 만들어 주는 이유는 추후 dag 를 순회할때 동시성(concurrency) 관점과 밀접한 관련이 있어 해당 부분에서 설명한다.(명령어 실행방식 참고) 끝 노드(endNode)의 경우는 파이프라인이 작업을 정상적으로 종료했을때를 알려주기 위한 역활을 가지고 있다.

2. dag 는 비순환을 특징으로 하고 있다. 따라서, 파이프라인이 정확히 순환인지 아닌지를 판단해야한다. 순환일 경우는 에러를 리턴해야 한다.

## 명령어 실행 방식
비록 명령어들이 dag 형태로 구성이 되었지만 명령어들을 실행하는 방식은 dag 순회 방식과는 다르다. 그 순회 방식 깊이 우선 또는 너비 우선을 따르지 않는다는 의미이다.

또한, [Indegree](https://url.kr/98g5bz) 를 이용하여 명령어들을 수행하는 [방식]( https://url.kr/zsh6ej )도 있으며 다양한 방식들이 구글링([1](https://url.kr/9spaz7), [2](https://url.kr/x5no4a)) 하면 나온다.

하지만 이러한 방식들은 golang 이라는 언어의 특징을 이용하는데 제한이 있는데, 아래와 같은 파이프라인이 존재한다고 한다면,
```
                -> B
startNode -> A          -> endNode
                -> C
```
A 노드가 실행을 완료했을때 그 결과를 가지고 B 와 C 는 동시에 실행할 수 있어야 한다. 그러나, 예를 들었던 방식들은 B 또는 C 를 순차적으로 실행하는 방식들이다. golang 이라는 언어를 사용한다면 B 와 C 는 동시에 실행되어야 한다.

이를 위해 두가지 아이디어를 적용했다.

1. edge 를 채널로 만들었다.
2. 파이프라인을 시작할때 모든 명령어들은 실행 상태로 만들고, 채널을 통해서 부모노드가 정상 종료 될때 채널을 통해서 종료 신호를 받고 실행을 완료 할 수 있도록 한다. 예를 들어 도미노를 쓰러트리는 방식으로 이해하면 이해하기 편하다.

### 도미노 쓰러트리기
위에서 간단히 언급한 도미노를 쓰러트리는 방식과 비슷하다고 설명을 하였는데, 도미노을 쓰러트리기 위해서는 먼저 모든 도미노 조각들을 세우는 과정들이 필요하다. 여기서 도미노 조각은 파이프라인을 구성하는 Node 와 같다고 이해하면 된다.

그리고 여기서 도미노 조각들을 세운다라는 개념은 Node 를 실행한다(정확히 표현하면 노드에 있는 익명함수를 고루틴으로 실행한다 이지만, Node 를 실행한다로 표현한다.)라는 의미이다.

이때 실행된 노드들은 실제로는 실행이 되지 않고 멈춰 있게 된다. '멈춰 있게 된다' 라는 개념은 아래 코드에서 해당 함수가 멈춰 있는 것과 같은 의미이다.

```
func (j *JobManSrv) Subscribe(in *pb.JobsRequest, s pb.LongLivedJobCall_SubscribeServer) error {

	fin := make(chan bool)

	// map 에 저장한다.
	j.subscribers.Store(in.JobReqId, sub{stream: s, finished: fin})
	ctx := s.Context()

	// TODO 11/6 추후 수정 asap. 테스트 코드를 만들어서 진행 후 적용
	cmd, r := j.scriptRunner(ctx, in)

	// 별도의 스레드로 실행해야  shell script 가 완료된후 시작하지 않는다.
	// 여기서 사용된 error 는 리턴 되지 않는다.
	go func(cmd *exec.Cmd) {
		if cmd != nil {
			if err := cmd.Start(); err != nil {
				log.Printf("Error starting Cmd: %v", err)
				return
			}
			if err := cmd.Wait(); err != nil {
				log.Printf("Error waiting for Cmd: %v", err)
				return
			}
		}
	}(cmd)

	// TODO error prone file already closed. 이녀석도 별도의 스레드로 돌린다.
	go j.reply(r)

	for {
		select {
		case <-fin:
			log.Printf("Closing stream for client ID: %d", in.JobReqId)
			return nil
		case <-ctx.Done():
			log.Printf("Client ID %d has disconnected", in.JobReqId)
			return nil
		}
	}
}
``` 

위의 코드는 github 에서 다른 프로젝트에서 작성한 코드인데 grpc 에서 long-lived call 방식을 구현한 방식이다.

해당 메서드에서 맨 아래 부분에서 실제로 해당 메서드를 실행시킬때 실행은 되지만 완료가 되지 않게 해주는 부분이다.

```
for {
		select {
		case <-fin:
			log.Printf("Closing stream for client ID: %d", in.JobReqId)
			return nil
		case <-ctx.Done():
			log.Printf("Client ID %d has disconnected", in.JobReqId)
			return nil
		}
	}

```

즉, 채널로 부터 특정 값이 들어오지 않으면 해당 부분에서 멈춰있다.

이러한 방식은 서버쪽에서 그 결과를 리턴하기 위해 대단히 많은 계산 시간을 필요로 하는 작업을 처리하기 위한 api 를 만들때 추천되는 [방식](https://url.kr/cdkqns)이다.

위와 비슷한 방식으로 아래와 같이 구현을 하였다.

먼저, 도미노 조각을 준비한다.
```
func setFunc(n *Node) {
	n.runner = func(n *Node, result chan printStatus) {
		//defer close(result)  // TODO 처리 해야 함. 최종적으로 채널에 보내는 모든 작업이 끝나면 RunningStatus chan printStatus close 를 실행해줘야 함.

		r := preFlight(n)
		result <- r

		r = inFlight(n)
		result <- r

		r = postFlight(n)
		result <- r
	}
}

func preFlight(n *Node) printStatus {
	// TODO 일단 nodeId 를 0 으로 처리함. 소스 정리 필요
	if n == nil {
		return printStatus{PreflightFailed, "0"}
	}
	i := len(n.parentVertex) // 부모 채널의 수
	wg := new(sync.WaitGroup)
	for j := 0; j < i; j++ {
		wg.Add(1)
		go func(c chan int) {
			defer wg.Done()
			<-c
			close(c)

		}(n.parentVertex[j])
	}
	wg.Wait() // 모든 고루틴이 끝날 때까지 기다림

	return printStatus{Preflight, n.Id}
}

func inFlight(n *Node) printStatus {

	if n == nil {
		return printStatus{InFlightFailed, "0"}
	}

	//if strings.Contains(n.Id, "end_node") {
	if n.Id == EndNode {
		fmt.Println(n.Id)
	}

	var bResult = false
	// TODO "end_node" 도 해야함.
	//if strings.Contains(n.Id, StartNode) || strings.Contains(n.Id, "end_node"){
	if n.Id == StartNode || n.Id == EndNode {
		bResult = true
	} else { // TODO debug 모드때문에 넣어 놓았음. AddEdge 하면 commands. 안들어감. 추후 삭제하거나, 다른 방향으로 작성해야함.
		if len(strings.TrimSpace(n.commands)) == 0 {
			bResult = true
		} else {
			bResult = shellexecmd.Runner(n.commands)
		}
	}

	if bResult {
		return printStatus{InFlight, n.Id}
	} else {
		return printStatus{InFlightFailed, n.Id}
	}
}

func postFlight(n *Node) printStatus {

	if n == nil {
		return printStatus{PostFlightFailed, "0"}
	}

	k := len(n.childrenVertex)
	for j := 0; j < k; j++ {
		n.childrenVertex[j] <- 1
	}
	//if strings.Contains(n.Id, "end_node") {
	if n.Id == EndNode {
		return printStatus{FlightEnd, n.Id}
	}
	return printStatus{PostFlight, n.Id}
}

func (dag *Dag) dagSetFunc() {

	n := len(dag.nodes)
	if n < 1 {
		return
	}

	for _, v := range dag.nodes {
		setFunc(v)
	}
}
```
다음으로, 도미노 조각도을 세운다. (실행시키지만 멈춰있게된다.)

```
func (dag *Dag) getReady() bool {
	n := len(dag.nodes)
	if n < 1 {
		return false
	}

	for _, v := range dag.nodes {
		go v.runner(v, dag.RunningStatus)
	}

	return true
}
```

startNode 를 완료했다는 신호를 보내준다. 그러면 세워진 도미노 조각들은 차례데로 넘어진다.(수행을 완료 또는 실제로 수행한다.)

```
func (dag *Dag) start() bool {
	n := len(dag.startNode.parentVertex)
	// 1 이 아니면 에러다.
	if n != 1 {
		return false
	}

	go func(c chan int) {
		ch := c
		ch <- 1
	}(dag.startNode.parentVertex[0])

	return true
}
```
신호를 보내 준다라는 의미는 채널에 값을 보낸다라는 의미이다.

### Edge 와 Vertex

## dag-go 구성 형태(추가 예정)
- dag.go 에는 dag 를 구성하게 해주는 주요 api 로 구성된다.

- parser.go 에는 dag.go 에 있는 api 를 이용하여 xml 을 파싱하고 이것을 통해서 pipeline 을 구성한다.

## xml 문법(추가 및 수정 예정)
```

<nodes>
		<node id = "1">
			<to>2</to>
			<command> echo "hello world 1"</command>
		</node>
		<node id = "2" >
			<from>1</from>
			<to>3</to>
			<command>echo "hello world 2"</command>
		</node>
		<node id ="3" >
			<from>2</from>
			<command>echo "hello world 3"</command>
		</node>
	</nodes>

```

### 우선 작업 진행중 (시간 소요 예정- podman 적용)

0. 각 node 들은 컨테이너 환경에서 독립적으로 실행 되어야 한다. 따라서, 컨테이너 이미지를 xml 에서 담아야 한다.
   https://podman.io/blogs/2020/08/10/podman-go-bindings.html

1. xml 구체적인 설계

각 단계에서 필요한 출력 및 필요 데이터 파일 연계 가능하게 하여야함.

xml 에서 파싱을 해서 가져오는 것은 가능하다. 다음으로, 처리해야 하는 것은 여러개의 데이터를 하나의 dag 를 이용해서 여러개의 pipeline 을 만드는 과정이 남았다.

이것은 xml 을 설계할때 부터 생각을 해야 하는 문제이다.



### TODO
2. 데이터 처리 메서드들 context 넣기, 및 취소, 일시 중지 기능 넣기.
3. 일시 중지는 특정 단계에서 넣었으면 해당 단계는 처리하고 다음단계에서 멈춤, 재시작 가능.(pipeline.go 에서 처리)
6. task 는 컨테이너에서 실행되어야 한다.
7. parser.go 코드 정리 및 기타 코드 정리
10. detectCycle, cloneGraph 버그 수정
11. https://hanpama.dev/posts/golang-org-x-sync
12. status 출력하는 메서드 
13. addedge 등 여러 메서들의 error 를 어떻게 관리할지 고민. - 일단 구현했는지 생각을 해봐야 할듯.
14. api 호출 순서 및 조정 필요.
15. time.duration 이 전역적으로 적용되고 적용된 값에서 전체적으로 감소하여서 적용될 수 있도록 한다.


### Done
1. task 수행 메서드 또는 함수 따로 빼내는 작업 처리. (4/20 구현 완료- 오류 및 성능 테스트 진행해야 함.)

### 시간날때 퍼포먼스 테스트
EndNode == "end_node" vs strings.Contains(n.Id, "end_node"), 일단, EndNode == "end_node" 로 바꿈.

## 채널
https://velog.io/@moonyoung/golang-channel-with-select-%ED%97%B7%EA%B0%88%EB%A6%AC%EB%8A%94-%EC%BC%80%EC%9D%B4%EC%8A%A4-%EC%A0%95%EB%A6%AC%ED%95%98%EA%B8%B0

