package dag_go

import (
	"context"
	"math/rand"
	"testing"
	"time"
)

// setupNode 는 테스트를 위한 Node 와 parentVertex 채널들을 설정
func setupNode(id string, numParents int, value runningStatus) *Node {
	node := &Node{Id: id}
	// numParents 개수만큼 부모 채널 생성 후 값을 비동기적으로 삽입
	node.parentVertex = make([]chan runningStatus, numParents)
	for i := 0; i < numParents; i++ {
		ch := make(chan runningStatus, 1)
		// 비동기적 값을 넣어서 실제 환경을 모사 (랜덤 딜레이 추가)
		go func(c chan runningStatus) {
			time.Sleep(time.Duration(rand.Intn(5)) * time.Millisecond)
			c <- value
		}(ch)
		node.parentVertex[i] = ch
	}
	return node
}

func BenchmarkPreFlight(b *testing.B) {
	ctx := context.Background()
	// 예를 들어 부모 채널이 10개인 노드, 모두 Succeed 를 보내도록 설정
	node := setupNode("benchmark_preFlight", 10, Succeed)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = preFlight(ctx, node)
		// 벤치마크를 반복 실행할 때, 상태를 초기화해야 할 수도 있음
		node.succeed = false
	}
}

func BenchmarkPreFlightCombined(b *testing.B) {
	ctx := context.Background()
	// 예를 들어 부모 채널이 10개인 노드, 모두 Succeed 를 보내도록 설정
	node := setupNode("benchmark_preFlightCombined", 10, Succeed)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = preFlightCombined(ctx, node)
		// 상태 초기화: preFlightCombined 내부에서 n.succeed 를 변경하기 때문에 초기 상태로 복구
		node.succeed = false
	}
}
