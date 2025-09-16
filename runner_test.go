package dag_go

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

type (
	echoOK   struct{ d time.Duration }
	echoFail struct{ d time.Duration }
)

func (r echoFail) RunE(a interface{}) error {
	id := "<unknown>"
	if n, ok := a.(*Node); ok && n != nil {
		id = n.ID
	}
	fmt.Printf("[RunE FAIL] node=%s sleep=%s\n", id, r.d)
	time.Sleep(r.d)
	return errors.New("mock failure")
}

func (r echoOK) RunE(a interface{}) error {
	id := "<unknown>"
	if n, ok := a.(*Node); ok && n != nil {
		id = n.ID
	}
	fmt.Printf("[RunE OK] node=%s sleep=%s\n", id, r.d)
	time.Sleep(r.d)
	return nil
}

func Test_lateBinding(t *testing.T) {
	dag, _ := InitDag()

	// 기본 러너: 전역 성공 목
	dag.SetContainerCmd(echoOK{d: 100 * time.Millisecond})

	// 노드 생성 및 엣지 구성
	a := dag.CreateNode("A")
	b := dag.CreateNode("B")
	_ = dag.AddEdge(StartNode, a.ID)
	_ = dag.AddEdge(a.ID, b.ID)

	// Resolver 등록: 특정 이미지/ID 규칙으로 동적 러너 선택 가능
	dag.SetRunnerResolver(func(n *Node) Runnable {
		if n.ID == "B" {
			return echoOK{d: 300 * time.Millisecond}
		}
		return nil
	})

	// 실행 준비
	_ = dag.FinishDag()
	_ = dag.ConnectRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = dag.GetReady(ctx)

	// (3) 상태 기반 허용: 아직 a가 실행 중이라면 b 러너를 바꿔도 반영됨
	dag.SetNodeRunner("B", echoFail{d: 150 * time.Millisecond})

	_ = dag.Start()
	_ = dag.Wait(ctx)
}
