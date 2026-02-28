package dag_go

import (
	"context"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"
)

// TestPreFlight_AllSucceed tests preFlight when all parent channels send Succeed.
func TestPreFlight_AllSucceed(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx := context.Background()

	node := &Node{ID: "node1"}

	// 부모 SafeChannel 2개 생성 (버퍼 크기 1)
	safeCh1 := NewSafeChannelGen[runningStatus](1)
	safeCh2 := NewSafeChannelGen[runningStatus](1)

	// 각 SafeChannel 에 Succeed 값을 전송
	if !safeCh1.Send(Succeed) {
		t.Fatal("Failed to send Succeed to safeCh1")
	}
	if !safeCh2.Send(Succeed) {
		t.Fatal("Failed to send Succeed to safeCh2")
	}

	// 부모 채널 슬라이스에 할당
	node.parentVertex = []*SafeChannel[runningStatus]{safeCh1, safeCh2}

	ps := preFlight(ctx, node)
	if ps.rStatus != Preflight {
		t.Errorf("Expected status %v, got %v", Preflight, ps.rStatus)
	}
	if ps.nodeID != node.ID {
		t.Errorf("Expected node id %s, got %s", node.ID, ps.nodeID)
	}
	if !node.IsSucceed() {
		t.Error("Expected node.succeed to be true")
	}
}

// TestPreFlight_OneFailed tests preFlight when one parent channel sends Failed.
func TestPreFlight_OneFailed(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx := context.Background()

	node := &Node{ID: "node2"}

	// 부모 SafeChannel 2개 생성 (버퍼 크기 1)
	safeCh1 := NewSafeChannelGen[runningStatus](1)
	safeCh2 := NewSafeChannelGen[runningStatus](1)

	// safeCh1에는 Succeed, safeCh2에는 Failed 값을 전송
	if !safeCh1.Send(Succeed) {
		t.Fatal("Failed to send Succeed to safeCh1")
	}
	if !safeCh2.Send(Failed) {
		t.Fatal("Failed to send Failed to safeCh2")
	}

	// 부모 채널 슬라이스에 SafeChannel 포인터들을 할당
	node.parentVertex = []*SafeChannel[runningStatus]{safeCh1, safeCh2}

	ps := preFlight(ctx, node)
	if ps.rStatus != PreflightFailed {
		t.Errorf("Expected status %v, got %v", PreflightFailed, ps.rStatus)
	}
	if ps.nodeID != "node2" {
		t.Errorf("Expected node id %s, got %s", noNodeID, ps.nodeID)
	}
	// 실패하였으므로 node 의 succeed 플래그는 false 여야 한다.
	if node.IsSucceed() {
		t.Error("Expected node.succeed to be false")
	}
}

// TestPreFlight_NoParents tests preFlight when there are no parent channels.
func TestPreFlight_NoParents(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx := context.Background()

	node := &Node{ID: "node5"}
	// 부모 채널이 없는 경우, 빈 SafeChannel 슬라이스 할당
	node.parentVertex = []*SafeChannel[runningStatus]{}

	ps := preFlight(ctx, node)
	if ps.rStatus != Preflight {
		t.Errorf("Expected status %v, got %v", Preflight, ps.rStatus)
	}
	if ps.nodeID != node.ID {
		t.Errorf("Expected node id %s, got %s", node.ID, ps.nodeID)
	}
	if !node.IsSucceed() {
		t.Error("Expected node.succeed to be true")
	}
}

// TestPreFlight_ContextCancelled tests preFlight when the context is canceled.
func TestPreFlight_ContextCanceled(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx, cancel := context.WithCancel(context.Background())
	node := &Node{ID: "node6"}

	// 부모 SafeChannel 하나 생성 (버퍼 크기 1). 값을 보내지 않아 블록 상태 유도.
	safeCh := NewSafeChannelGen[runningStatus](1)
	node.parentVertex = []*SafeChannel[runningStatus]{safeCh}

	// 컨텍스트 취소
	cancel()

	done := make(chan *printStatus)
	go func() {
		done <- preFlight(ctx, node)
	}()

	select {
	case ps := <-done:
		if ps.rStatus != PreflightFailed {
			t.Errorf("Expected status %v due to context cancellation, got %v", PreflightFailed, ps.rStatus)
		}
		if node.IsSucceed() {
			t.Error("Expected node.succeed to be false after context cancellation")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("preFlight did not return within timeout; context cancellation not handled")
	}
}

// TestPreFlight_AllSucceed_WithManyChannels tests preFlight with many asynchronous parent channels.
func TestPreFlight_AllSucceed_WithManyChannels(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx := context.Background()

	node := &Node{ID: "node_async"}
	// 예를 들어, 10개의 부모 채널을 비동기적으로 값 넣도록 생성
	node.parentVertex = createParentChannels(10, Succeed)

	ps := preFlight(ctx, node)
	if ps.rStatus != Preflight {
		t.Errorf("Expected status %v, got %v", Preflight, ps.rStatus)
	}
	if ps.nodeID != node.ID {
		t.Errorf("Expected node id %s, got %s", node.ID, ps.nodeID)
	}
	if !node.IsSucceed() {
		t.Error("Expected node.succeed to be true")
	}
}

// ── TransitionStatus unit tests ──────────────────────────────────────────────

// TestTransitionStatus_ValidTransitions verifies that every permitted edge in
// the NodeStatus state machine is accepted by TransitionStatus.
func TestTransitionStatus_ValidTransitions(t *testing.T) {
	defer goleak.VerifyNone(t)

	cases := []struct {
		name string
		from NodeStatus
		to   NodeStatus
	}{
		{"Pending→Running", NodeStatusPending, NodeStatusRunning},
		{"Pending→Skipped", NodeStatusPending, NodeStatusSkipped},
		{"Running→Succeeded", NodeStatusRunning, NodeStatusSucceeded},
		{"Running→Failed", NodeStatusRunning, NodeStatusFailed},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := createNodeWithID("v-" + tc.name)
			// Force the node into the `from` state before testing the transition.
			n.SetStatus(tc.from)
			if !n.TransitionStatus(tc.from, tc.to) {
				t.Errorf("expected TransitionStatus(%v→%v) to return true, got false", tc.from, tc.to)
			}
			if got := n.GetStatus(); got != tc.to {
				t.Errorf("expected status %v after transition, got %v", tc.to, got)
			}
		})
	}
}

// TestTransitionStatus_InvalidTransitions verifies that illegal state-machine
// edges are always rejected, preventing backwards or terminal-state overwrites.
func TestTransitionStatus_InvalidTransitions(t *testing.T) {
	defer goleak.VerifyNone(t)

	cases := []struct {
		name    string
		initial NodeStatus
		from    NodeStatus
		to      NodeStatus
	}{
		// Wrong pre-condition (from ≠ current).
		{"Pending:RunningAsFrom", NodeStatusPending, NodeStatusRunning, NodeStatusSucceeded},
		// Backwards transition.
		{"Running→Pending", NodeStatusRunning, NodeStatusRunning, NodeStatusPending},
		// Terminal → any.
		{"Failed→Succeeded", NodeStatusFailed, NodeStatusFailed, NodeStatusSucceeded},
		{"Succeeded→Running", NodeStatusSucceeded, NodeStatusSucceeded, NodeStatusRunning},
		{"Skipped→Running", NodeStatusSkipped, NodeStatusSkipped, NodeStatusRunning},
		// Already skipped, attempt duplicate Pending→Skipped.
		{"Skipped→Skipped", NodeStatusSkipped, NodeStatusPending, NodeStatusSkipped},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := createNodeWithID("inv-" + tc.name)
			n.SetStatus(tc.initial)
			if n.TransitionStatus(tc.from, tc.to) {
				t.Errorf("expected TransitionStatus(%v→%v) to return false (initial=%v), got true",
					tc.from, tc.to, tc.initial)
			}
			// Status must remain unchanged.
			if got := n.GetStatus(); got != tc.initial {
				t.Errorf("expected status %v to be unchanged, got %v", tc.initial, got)
			}
		})
	}
}

// TestTransitionStatus_ConcurrentPendingToRunning launches many goroutines that
// all race to perform the Pending→Running transition on a single node.
// Exactly one must win; all others must be rejected.  Run with -race to verify
// there are no data races on the status field.
func TestTransitionStatus_ConcurrentPendingToRunning(t *testing.T) {
	defer goleak.VerifyNone(t)

	const numGoroutines = 200
	n := createNodeWithID("cas-race")

	var (
		wg   sync.WaitGroup
		wins int32
	)

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ { //nolint:intrange
		go func() {
			defer wg.Done()
			if n.TransitionStatus(NodeStatusPending, NodeStatusRunning) {
				atomic.AddInt32(&wins, 1)
			}
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&wins); got != 1 {
		t.Errorf("expected exactly 1 Pending→Running winner, got %d", got)
	}
	if n.GetStatus() != NodeStatusRunning {
		t.Errorf("expected node status Running after CAS race, got %v", n.GetStatus())
	}
}

// TestTransitionStatus_ConcurrentFullLifecycle runs a complete Pending→Running→
// {Succeeded|Failed} lifecycle under concurrent pressure from goroutines that
// attempt duplicate or illegal transitions at each phase.
//
//nolint:gocognit // multi-phase concurrent test; complexity comes from coordinating three distinct race phases
func TestTransitionStatus_ConcurrentFullLifecycle(t *testing.T) {
	defer goleak.VerifyNone(t)

	const attackers = 100
	n := createNodeWithID("lifecycle-race")

	// Phase 1: race Pending → Running.
	var wg sync.WaitGroup
	var phase1Wins int32
	wg.Add(attackers)
	for i := 0; i < attackers; i++ { //nolint:intrange
		go func() {
			defer wg.Done()
			if n.TransitionStatus(NodeStatusPending, NodeStatusRunning) {
				atomic.AddInt32(&phase1Wins, 1)
			}
		}()
	}
	wg.Wait()

	if atomic.LoadInt32(&phase1Wins) != 1 {
		t.Fatalf("phase1: expected 1 Pending→Running winner, got %d", phase1Wins)
	}

	// Phase 2: race Running → Succeeded (half goroutines) vs Running → Failed (other half).
	var (
		succeedWins int32
		failedWins  int32
	)
	wg.Add(attackers)
	for i := 0; i < attackers; i++ { //nolint:intrange
		go func(idx int) {
			defer wg.Done()
			if idx%2 == 0 {
				if n.TransitionStatus(NodeStatusRunning, NodeStatusSucceeded) {
					atomic.AddInt32(&succeedWins, 1)
				}
			} else {
				if n.TransitionStatus(NodeStatusRunning, NodeStatusFailed) {
					atomic.AddInt32(&failedWins, 1)
				}
			}
		}(i)
	}
	wg.Wait()

	totalPhase2 := atomic.LoadInt32(&succeedWins) + atomic.LoadInt32(&failedWins)
	if totalPhase2 != 1 {
		t.Errorf("phase2: expected exactly 1 terminal transition, got %d (succeed=%d, failed=%d)",
			totalPhase2, atomic.LoadInt32(&succeedWins), atomic.LoadInt32(&failedWins))
	}

	// Phase 3: terminal state must reject all further transitions.
	final := n.GetStatus()
	if final != NodeStatusSucceeded && final != NodeStatusFailed {
		t.Errorf("expected terminal status, got %v", final)
	}

	var phase3Wins int32
	wg.Add(attackers)
	for i := 0; i < attackers; i++ { //nolint:intrange
		go func() {
			defer wg.Done()
			// Both of these must be rejected because final is a terminal state.
			if n.TransitionStatus(NodeStatusSucceeded, NodeStatusRunning) ||
				n.TransitionStatus(NodeStatusFailed, NodeStatusRunning) {
				atomic.AddInt32(&phase3Wins, 1)
			}
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&phase3Wins); got != 0 {
		t.Errorf("phase3: expected 0 transitions from terminal state, got %d", got)
	}
}

// createParentChannels 는 n개의 채널을 생성하고, 각 채널에 비동기적으로 주어진 value 를 전송
func createParentChannels(n int, value runningStatus) []*SafeChannel[runningStatus] {
	channels := make([]*SafeChannel[runningStatus], n)
	for i := 0; i < n; i++ { //nolint:intrange
		// SafeChannel 생성 (버퍼 크기 1)
		sc := NewSafeChannelGen[runningStatus](1)
		go func(sc *SafeChannel[runningStatus]) {
			// 랜덤 딜레이 (0 ~ 10밀리초)로 실제 환경의 비동기성을 모사
			time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond) // #nosec G404 -- acceptable for test timing jitter
			sc.Send(value)
		}(sc)
		channels[i] = sc
	}
	return channels
}
