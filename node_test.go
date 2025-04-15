package dag_go

import (
	"context"
	"go.uber.org/goleak"
	"math/rand"
	"testing"
	"time"
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
