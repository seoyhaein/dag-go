package scheduler

import (
	"sync"

	"golang.org/x/sync/singleflight"
)

// TODO 코드 어느정도 완성되면 주석 다 지움. 기록 보관용으로 주석은 설명 자료 참고용으로 넘김.
// ===== Dispatcher =====

type Dispatcher struct {
	// registry 읽을 때는 RLock(), 새 액터 생성/등록 시 Lock()으로 보호.
	// 다수 읽기 동시 허용, 쓰기 시 단독 진입.
	regMu sync.RWMutex

	// actor 레지스트리, spawnId -> actor, 현재 살아있는 액터들을 관리하는 테이블
	registry Registry

	// 멱등키 맵(idemToSpan) 보호용 락.
	// regMu와 분리해 락 경합 줄임(멱등키 조회는 상대적으로 자주 일어남).
	// 정해진 락 순서: 가능하면 idemMu → regMu 순으로 잡아 데드락 회피.
	idemMu sync.Mutex

	// IdemKey → spawnId 매핑(멱등성 구현).
	// 동일 IdemKey로 들어오면 같은 spawnId를 반환해 중복 실행/경쟁 회피.
	// 없으면 새 spawnId를 만들고 맵에 기록.
	// 재연결/재시도 시 기존 실행에 재구독 같은 고급 시나리오의 기초가 됨.
	// idemKey -> spawnId (for idempotency)
	idemToSpan map[string]string

	// 전역 동시 실행 제한 세마포어.
	// 버퍼 크기 = 동시에 실행 허용할 “액터 수(또는 런 수)”.
	// DispatchRun에서 시작 시 sem <- struct{}{}로 슬롯 획득, 액터가 터미널 상태 시 반납.
	// 포화 시 정책: 지금은 에러 반환(스파이크 단계). 실제에선 대기/큐잉/우선순위/리젝트 등 선택.
	// global concurrency gate
	sem chan struct{}

	// 액터 생성 DI(의존성 주입) 훅.
	// 테스트에서 가짜 Actor 주입, 메트릭/옵저버블리티 훅 삽입, 다른 구현(예: PodActor/ContainerActor) 스왑 가능.
	// 기본값은 NewActor.
	// DI for testing
	newActor func(spawnID string) *Actor

	// ensure only one creator runs per spawnID under contention
	// golang.org/x/sync@v0.12.0 에서 golang.org/x/sync v0.17.0 업데이트 하고 벤더링 함.
	sf singleflight.Group
}

func NewDispatcher(maxConcurrent int, reg Registry) *Dispatcher {
	if reg == nil {
		reg = NewInMemoryRegistry()
	}
	return &Dispatcher{
		registry:   reg,
		idemToSpan: make(map[string]string),
		sem:        make(chan struct{}, maxConcurrent),
		newActor:   func(spawnID string) *Actor { return NewActor(spawnID) },
	}
}

func (d *Dispatcher) getOrCreateActor(spawnID string) *Actor {
	// Fast-path read from registry
	if act, ok := d.registry.Get(spawnID); ok {
		return act
	}
	// Under contention, only one goroutine performs creation per spawnID
	v, _, _ := d.sf.Do(spawnID, func() (interface{}, error) {
		// Double-check inside the singleflight critical section
		if existing, ok := d.registry.Get(spawnID); ok {
			return existing, nil
		}
		a := d.newActor(spawnID)
		d.registry.Put(spawnID, a)
		go a.loop()
		return a, nil
	})
	if a, ok := v.(*Actor); ok {
		return a
	}
	return nil // should not happen
}
