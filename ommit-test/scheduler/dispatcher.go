package scheduler

import (
	"errors"
	"fmt"
	"sync"

	"golang.org/x/sync/singleflight"
)

type Dispatcher struct {

	// 전역 동시 실행 제한 세마포어.
	// 버퍼 크기 = 동시에 실행 허용할 “액터 수(또는 런 수)”.
	// DispatchRun에서 시작 시 sem <- struct{}{}로 슬롯 획득, 액터가 터미널 상태 시 반납.
	// 포화 시 정책: 지금은 에러 반환(스파이크 단계). 실제에선 대기/큐잉/우선순위/리젝트 등 선택.
	// global concurrency gate
	sem chan struct{} // slot=actor: simultaneously alive actors upper bound

	// 라우터...
	router *Router

	actors sync.Map // sync.Map keeps registry ops lock-free-ish
	// TODO 일단 해보자.
	registry  sync.Map
	registry1 Registry

	// 멱등키 맵(idemToSpan) 보호용 락.
	// regMu와 분리해 락 경합 줄임(멱등키 조회는 상대적으로 자주 일어남).
	// 정해진 락 순서: 가능하면 idemMu → regMu 순으로 잡아 데드락 회피.
	idemMu sync.Mutex

	// IdemKey → spawnId 매핑(멱등성 구현).
	// 동일 IdemKey로 들어오면 같은 spawnId를 반환해 중복 실행/경쟁 회피.
	// 없으면 새 spawnId를 만들고 맵에 기록.
	// 재연결/재시도 시 기존 실행에 재구독 같은 고급 시나리오의 기초가 됨.
	// idemKey -> spawnId (for idempotency)
	idemToSpawn map[string]string

	// ensure only one creator runs per spawnID under contention
	// golang.org/x/sync@v0.12.0 에서 golang.org/x/sync v0.17.0 업데이트 하고 벤더링 함.
	sf singleflight.Group // optional but handy when creation gets heavy

	// spawnID = RunID + "/" + NodeID
	// 액터 생성 DI(의존성 주입) 훅.
	// 테스트에서 가짜 Actor 주입, 메트릭/옵저버블리티 훅 삽입, 다른 구현(예: PodActor/ContainerActor) 스왑 가능.
	// 기본값은 NewActor.
	// DI for testing
	newActor func(spawnID string) *Actor // default: NewActor1(...)

	// 테스트 관찰용 훅
	onActorCreate func(spawnID string)
}

func NewDispatcher(maxActors int, router *Router) *Dispatcher {
	return &Dispatcher{
		sem:    make(chan struct{}, maxActors),
		router: router,
	}
}

func NewDispatcher1(maxConcurrent int, reg Registry) *Dispatcher {
	if reg == nil {
		reg = NewInMemoryRegistry()
	}
	return &Dispatcher{
		registry1:   reg,
		idemToSpawn: make(map[string]string),
		sem:         make(chan struct{}, maxConcurrent),
		newActor:    func(spawnID string) *Actor { return NewActor(spawnID) },
	}
}

func (d *Dispatcher) getOrCreateActor(spawnID string) (*Actor, error) {
	if v, ok := d.registry.Load(spawnID); ok {
		return v.(*Actor), nil
	}
	// create path
	select {
	case d.sem <- struct{}{}:
		// got slot
	default:
		return nil, errors.New("dispatcher saturated: too many actors")
	}

	act := NewActor1(spawnID, d.router, 0, func() {
		<-d.sem
		d.registry.Delete(spawnID)
	})
	if actual, loaded := d.registry.LoadOrStore(spawnID, act); loaded {
		// 다른 고루틴이 먼저 만들었음 → 우리 슬롯 반납
		<-d.sem
		return actual.(*Actor), nil
	}
	// 최초 생성 알림 (테스트용)
	if d.onActorCreate != nil {
		d.onActorCreate(spawnID)
	}
	act.Start()
	return act, nil
}

func (d *Dispatcher) getOrCreateActor1(spawnID string) (*Actor, error) {
	// Fast-path: already exists
	if act, ok := d.registry1.Get(spawnID); ok {
		return act, nil
	}

	// Ensure only one creator runs per spawnID (singleflight v0.17.0: v, err, shared)
	// 동일 키에 대한 동시성 문제 발생시 해결
	v, err, _ := d.sf.Do(spawnID, func() (interface{}, error) {
		// Double-check inside critical section
		if existing, ok := d.registry1.Get(spawnID); ok {
			return existing, nil
		}
		// 세마포어
		// slot=actor: acquire a slot BEFORE creating the actor
		select {
		case d.sem <- struct{}{}:
		// acquired; proceed
		default:
			return nil, errors.New("dispatcher saturated: max concurrent actors (by actor) reached")
		}

		a := d.newActor(spawnID)
		if a == nil {
			// defensive: creation failed → release the slot we just took
			<-d.sem
			return nil, errors.New("newActor returned nil")
		}

		d.registry1.Put(spawnID, a)
		go a.loop()
		return a, nil
	})
	if err != nil {
		return nil, fmt.Errorf("getOrCreateActor(%s) failed: %w", spawnID, err)
	}

	act, ok := v.(*Actor)
	if !ok {
		return nil, errors.New("unexpected type from singleflight")
	}
	return act, nil
}

func (d *Dispatcher) DispatchRun(req RunReq, sink EventSink) error {
	spawnID := sessionKeyOf(req) // 정책: 노드 단위 직렬화 (RunID/NodeID)
	act, err := d.getOrCreateActor(spawnID)
	if err != nil {
		return err
	}
	cmd := Command{
		Kind:    CmdRun,
		RunReq:  req,
		SpawnID: spawnID,
		RunID:   req.RunID,
		NodeID:  req.NodeID,
		Sink:    sink,
	}
	if ok := act.enqueue(cmd); !ok {
		return errors.New("mailbox full")
	}
	return nil
}

func sessionKeyOf(m RunReq) string {
	return m.RunID + "/" + m.NodeID
}
