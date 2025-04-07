package dag_go

import (
	"context"
	"io"
	"math/rand"
	"testing"
	"time"
)

// 웜업 표준 만들자. 일단 모두 하는 걸로, 성능의 차이는 크게 없으므로.
// 채널 및 고루틴 사용 메서드의 경우 벤치마크 테스트 신중히 작성해야 함.
// 벤치마크 테스트 할 경우 fmt.Println() 등의 출력문을 사용하지 말 것. 로그를 사용하고 이 로그를 벤치마크 메서드에서 출력되지 않도록 해야함.

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
	// 로그가 있을 경우 출력결과물에 로그 기록이 남겨지는 것을 방지.
	Log.SetOutput(io.Discard)
	ctx := context.Background()
	// 예를 들어 부모 채널이 10개인 노드, 모두 Succeed 를 보내도록 설정
	node := setupNode("benchmark_preFlight", 10, Succeed)

	// Warm-up: 벤치마크 루프 전에 한 번 실행하고, 부모 채널 재채움
	_ = preFlight(ctx, node)
	// 각 부모 채널에 Succeed 값을 다시 삽입
	for j := 0; j < len(node.parentVertex); j++ {
		node.parentVertex[j] <- Succeed
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = preFlight(ctx, node)
		// 매 반복 후 부모 채널을 다시 채워 다음 반복에 대비
		for j := 0; j < len(node.parentVertex); j++ {
			node.parentVertex[j] <- Succeed
		}
	}
}
