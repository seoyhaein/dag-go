package scheduler

// executor 에서 RunE 를 실행할때, 여기서 api 를 호출하는 형태로 해야함. 지금은 바로 연결을 하지만, 내부적으로 향후 grpc 로 연결을 하려고함.
// RunE 형태의 설계 방향은 client 즉, executor 에서는 단일한 형태로 가지만, 서버쪽에서는 세부적으로 다양하게 api 들이 들어갈 수 있는 형태로 가야할 것 같다.
// Pod 만들기, 호스트 overlay, 컨테이너 만들기, 리소스 해제 등등의 api 들이 들어갈 수 있을 것 같다.
// podspawner.go 에서는 podbridge5 내용이 들어와야 하는 구조이다.
// 디스패처, 액터 모델 생각해보기.
// 메세지 형태로 전송하고 이것을 받아서 처리하는 형태로 간다.

/*var dispatcher *Dispatcher

func init() {
	// 일단 그냥 작성함. 에러 신경안씀.
	dispatcher = NewDispatcherWithMemory(100) // 버퍼 100개짜리 디스패처 생성

}*/

// 생각을 좀 다시 해야 할듯.
// RunE 구현은 executor 에서 해줘야 함.
// 여기서는 그냥 message 형태로 보내는 것에 한정함.
// 그것을 podspawner.go 에서 받아서 처리하는 형태로 해야 함.
// 이 메세지를 registry 에 저장해줘야 하고 이걸 podspawner.go 에서 꺼내서 처리하는 형태로 해야 함.
// 파이프라인 실행시, 노드별로 RunE 가 호출되면 이때, 메세지가 전달됨. 이때 전달되는 메세지의 경우 파이프라인의 하나의 실행 단위임.
// 이걸 하나 처리하는 것을 디스패치가 담당하게 할까? 리소스 문제도 생각해야 하는데...

/*func (r RunReq) RunE(a interface{}) error {
	id := "<unknown>"
	if n, ok := a.(*dag_go.Node); ok && n != nil {
		id = n.ID
	}
	fmt.Printf("[RunE OK] node=%s\n", id)
	// 여기서 채워서 보내야 함.
	// 노드에서 받은 정보를 RunReq 에 채워서 보내야 함.

	ctx := context.Background()
	// TODO runId 같은 경우는 파이프라인 실행시에 가져오는 Id 인데 일단 여기에서는 없다.
	rr := NewRunReq(id, id, id, nil)
	dispatcher.DispatchRun(ctx, rr)
	return nil
}*/
