package scheduler

import (
	"sync"
	"testing"
	"time"
)

// ===== Test =====

func Test_Dispatcher_CreatesTwoActors_ForParallelNodes(t *testing.T) {
	router := &Router{drv: &noopDriver{}} // (현재 테스트에선 미사용이지만 의존성 주입)
	d := NewDispatcher(8, router)

	created := make(chan string, 4)
	d.onActorCreate = func(spawnID string) {
		created <- spawnID
	}

	// A가 끝났다고 가정하고, B와 C를 "거의 동시에" 전달
	reqB := RunReq{RunID: "run-1", NodeID: "B", IdemKey: "run-1/B#1"}
	reqC := RunReq{RunID: "run-1", NodeID: "C", IdemKey: "run-1/C#1"}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := d.DispatchRun(reqB, StdoutSink{}); err != nil {
			t.Errorf("dispatch B: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		if err := d.DispatchRun(reqC, StdoutSink{}); err != nil {
			t.Errorf("dispatch C: %v", err)
		}
	}()
	wg.Wait()

	// 생성된 Actor 두 개를 기다린다
	timeout := time.After(1 * time.Second)
	got := map[string]bool{}
	for len(got) < 2 {
		select {
		case id := <-created:
			got[id] = true
		case <-timeout:
			t.Fatalf("timed out waiting for two actors; got=%v", got)
		}
	}

	if !got["run-1/B"] || !got["run-1/C"] {
		t.Fatalf("expected actors for B and C; got=%v", got)
	}

	t.Logf("created actors: %v", got)

	// (옵션) 약간 기다려서 FakeDriver 진행 로그가 찍히는 것을 볼 수 있음
	time.Sleep(100 * time.Millisecond)
}
