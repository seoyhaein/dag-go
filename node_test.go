package dag_go

import (
	"context"
	"math/rand"
	"testing"
	"time"
)

// TestCloneGraph: cloneGraph 함수 테스트 (기존 DAG 복사)
/*func TestCloneGraph(t *testing.T) {
	var copied map[string]*Node
	dag := NewDag()
	dag.AddEdge(dag.StartNode.Id, "1")
	dag.AddEdge("1", "2")
	dag.AddEdge("1", "3")
	dag.AddEdge("1", "4")
	dag.AddEdge("2", "5")
	dag.AddEdge("5", "6")
	dag.AddCommand("1", "")
	dag.FinishDag()

	copied, _ = cloneGraph(dag.nodes)

	nCopied := len(copied)
	nOriginal := len(dag.nodes)
	fmt.Println("원래 노드 수 (예: 8가 나와야 정상):", nOriginal)
	fmt.Println("복사된 노드 수 (예: 8가 나와야 정상):", nCopied)
}*/

// TestPreFlight_AllSucceed tests preFlight when all parent channels send Succeed.
func TestPreFlight_AllSucceed(t *testing.T) {
	ctx := context.Background()

	node := &Node{Id: "node1"}
	// 부모 채널 2개 생성 (버퍼 1)
	ch1 := make(chan runningStatus, 1)
	ch2 := make(chan runningStatus, 1)
	ch1 <- Succeed
	ch2 <- Succeed
	node.parentVertex = []chan runningStatus{ch1, ch2}

	ps := preFlight(ctx, node)
	if ps.rStatus != Preflight {
		t.Errorf("Expected status %v, got %v", Preflight, ps.rStatus)
	}
	if ps.nodeId != node.Id {
		t.Errorf("Expected node id %s, got %s", node.Id, ps.nodeId)
	}
	if !node.succeed {
		t.Error("Expected node.succeed to be true")
	}
}

// TestPreFlight_OneFailed tests preFlight when one parent channel sends Failed.
func TestPreFlight_OneFailed(t *testing.T) {
	ctx := context.Background()

	node := &Node{Id: "node2"}
	// 부모 채널 2개: 하나는 Succeed, 하나는 Failed
	ch1 := make(chan runningStatus, 1)
	ch2 := make(chan runningStatus, 1)
	ch1 <- Succeed
	ch2 <- Failed
	node.parentVertex = []chan runningStatus{ch1, ch2}

	ps := preFlight(ctx, node)
	if ps.rStatus != PreflightFailed {
		t.Errorf("Expected status %v, got %v", PreflightFailed, ps.rStatus)
	}
	if ps.nodeId != noNodeId {
		t.Errorf("Expected node id %s, got %s", noNodeId, ps.nodeId)
	}
	if node.succeed {
		t.Error("Expected node.succeed to be false")
	}
}

// TestPreFlightT_AllSucceed tests preFlightT when all parent channels send Succeed.
func TestPreFlightT_AllSucceed(t *testing.T) {
	ctx := context.Background()

	node := &Node{Id: "node3"}
	ch1 := make(chan runningStatus, 1)
	ch2 := make(chan runningStatus, 1)
	ch1 <- Succeed
	ch2 <- Succeed
	node.parentVertex = []chan runningStatus{ch1, ch2}

	ps := preFlight(ctx, node)
	if ps.rStatus != Preflight {
		t.Errorf("Expected status %v, got %v", Preflight, ps.rStatus)
	}
	if ps.nodeId != node.Id {
		t.Errorf("Expected node id %s, got %s", node.Id, ps.nodeId)
	}
	if !node.succeed {
		t.Error("Expected node.succeed to be true")
	}
}

// TestPreFlightT_OneFailed tests preFlightT when one parent channel sends Failed.
func TestPreFlightT_OneFailed(t *testing.T) {
	ctx := context.Background()

	node := &Node{Id: "node4"}
	ch1 := make(chan runningStatus, 1)
	ch2 := make(chan runningStatus, 1)
	ch1 <- Succeed
	ch2 <- Failed
	node.parentVertex = []chan runningStatus{ch1, ch2}

	ps := preFlight(ctx, node)
	if ps.rStatus != PreflightFailed {
		t.Errorf("Expected status %v, got %v", PreflightFailed, ps.rStatus)
	}
	if ps.nodeId != noNodeId {
		t.Errorf("Expected node id %s, got %s", noNodeId, ps.nodeId)
	}
	if node.succeed {
		t.Error("Expected node.succeed to be false")
	}
}

// TestPreFlight_NoParents tests preFlight when there are no parent channels.
func TestPreFlight_NoParents(t *testing.T) {
	ctx := context.Background()

	node := &Node{Id: "node5"}
	// 부모 채널이 없는 경우
	node.parentVertex = []chan runningStatus{}

	ps := preFlight(ctx, node)
	if ps.rStatus != Preflight {
		t.Errorf("Expected status %v, got %v", Preflight, ps.rStatus)
	}
	if ps.nodeId != node.Id {
		t.Errorf("Expected node id %s, got %s", node.Id, ps.nodeId)
	}
	if !node.succeed {
		t.Error("Expected node.succeed to be true")
	}
}

// TestPreFlight_ContextCancelled tests preFlight when the context is cancelled.
func TestPreFlight_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	node := &Node{Id: "node6"}
	// 부모 채널 하나 생성, 값을 보내지 않아 블록 상태 유도
	ch := make(chan runningStatus, 1)
	node.parentVertex = []chan runningStatus{ch}

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
		if node.succeed {
			t.Error("Expected node.succeed to be false after context cancellation")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("preFlight did not return within timeout; context cancellation not handled")
	}
}

// TestPreFlight_AllSucceed_WithManyChannels tests preFlight with many asynchronous parent channels.
func TestPreFlight_AllSucceed_WithManyChannels(t *testing.T) {
	ctx := context.Background()

	node := &Node{Id: "node_async"}
	// 예를 들어, 10개의 부모 채널을 비동기적으로 값 넣도록 생성
	node.parentVertex = createParentChannels(10, Succeed)

	ps := preFlight(ctx, node)
	if ps.rStatus != Preflight {
		t.Errorf("Expected status %v, got %v", Preflight, ps.rStatus)
	}
	if ps.nodeId != node.Id {
		t.Errorf("Expected node id %s, got %s", node.Id, ps.nodeId)
	}
	if !node.succeed {
		t.Error("Expected node.succeed to be true")
	}
}

// createParentChannels 는 n개의 채널을 생성하고, 각 채널에 비동기적으로 주어진 value 를 전송
func createParentChannels(n int, value runningStatus) []chan runningStatus {
	channels := make([]chan runningStatus, n)
	for i := 0; i < n; i++ {
		ch := make(chan runningStatus, 1)
		go func(c chan runningStatus) {
			// 랜덤 딜레이 (0~10밀리초)로 실제 환경의 비동기성을 모사
			time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)
			c <- value
		}(ch)
		channels[i] = ch
	}
	return channels
}
