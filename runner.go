package dag_go

// for late binding

import (
	"errors"
)

// ErrNoRunner 실패 방지 가드: 러너가 없으면 명시적 에러
var ErrNoRunner = errors.New("no runner set for node")

// RunnerResolver 선택적 Resolver: 노드 메타데이터로 동적 러너 결정
type RunnerResolver func(*Node) Runnable

// 고정 래퍼 타입 추가
type runnerSlot struct{ r Runnable }

// Node에 저장/조회 헬퍼 메서드 추가
func (n *Node) runnerStore(r Runnable) { // 항상 *runnerSlot만 저장
	n.runnerVal.Store(&runnerSlot{r: r})
}
func (n *Node) runnerLoad() Runnable {
	v := n.runnerVal.Load()
	if v == nil {
		return nil
	}
	return v.(*runnerSlot).r
}

// SetRunnerUnsafe Node/Runnable 저장을 원자적으로(동시성 안전), 안전한 동시성 처리: atomic.Value로 러너 보관
func (n *Node) SetRunnerUnsafe(r Runnable) { // 내부용(락 없이 쓰고 싶을 때)
	// 이건 안전하지 않음.
	n.RunCommand = r
	// 이건 안전함.
	n.runnerStore(r)
}

func (n *Node) getRunnerSnapshot() Runnable {
	// 두개의 값을 하나는 원자적으로 안정성을 보장하고, 하나는 RW 뮤텍스 걸어서 안정성을 보장함.
	// RW 로 할려고 했지만, 일단 그냥 이렇게 진행함.
	// 일단 먼저 노드 레벨에서 확인 설정이 되어 있으면 이걸 가져옴.
	// runnerVal atomic.Value 사용함으로 동시성 안전
	if r := n.runnerLoad(); r != nil {
		return r
	} // node-level override (atomic)

	// 없으면 DAG 레벨에서 확인
	// DAG 에서는 RW 뮤텍스 걸어서 동시성 안전

	// 우선 순위 : Node.runnerVal > Dag.RunnerResolver > Dag.ContainerCmd

	var rr RunnerResolver
	var base Runnable
	if d := n.parentDag; d != nil {
		d.mu.RLock()
		rr = d.runnerResolver
		// base 는 nil 일수도 있음. 그럼 ErrNoRunner 로 처리함. 그리고, 여기서 설정이 된다면, Node 의 RunCommand 필드가 사용되지 않음.
		base = d.ContainerCmd
		d.mu.RUnlock()
	}
	if rr != nil {
		if r := rr(n); r != nil {
			return r
		}
	}
	return base
}
