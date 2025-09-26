package scheduler

import (
	"sync/atomic"
	"testing"
)

func noLeakActor(spawnID string, counter *int32) *Actor {
	atomic.AddInt32(counter, 1)
	ch := make(chan Command)
	close(ch) // loop() will range over a closed channel and exit immediately
	return &Actor{spawnID: spawnID, mbox: ch}
}

// maxConcurrent는 동시에 유지 가능한 액터 수이므로, “노드/컨테이너 동시 실행 상한”과 맞춰 잡아줘.
//idemToSpan은 장기 운영 시 메모리 누수 방지 위해 TTL/청소 루틴을 붙이거나, 별도 IdemStore 인터페이스로 빼서 Redis 같은 외부 스토어로 이동하는 걸 추천.

func TestGetOrCreateActor01(t *testing.T) {
	reg := NewInMemoryRegistry()
	d := NewDispatcher1(100, reg)

	var created int32
	d.newActor = func(spawnID string) *Actor { return noLeakActor(spawnID, &created) }

	a, errA := d.getOrCreateActor1("sp_A")
	if errA != nil {
		t.Fatalf("unexpected error for sp_A: %v", errA)
	}
	b, errB := d.getOrCreateActor1("sp_B")
	if errB != nil {
		t.Fatalf("unexpected error for sp_B: %v", errB)
	}

	if a == nil || b == nil {
		t.Fatal("expected non-nil actors")
	}
	if a == b {
		t.Fatal("expected different *Actor pointers for different spawnIDs")
	}
	if got := atomic.LoadInt32(&created); got != 2 {
		t.Fatalf("expected 2 creations (A,B), got %d", got)
	}
}
