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
	// Node 구조체 생성 및 parentVertex 슬라이스 초기화
	node := &Node{
		ID:           id,
		parentVertex: make([]*SafeChannel[runningStatus], numParents),
	}

	// numParents 만큼 부모용 SafeChannel 생성 및 비동기적으로 값 전송
	for i := 0; i < numParents; i++ {
		// SafeChannel 생성 (버퍼 크기 1)
		sc := NewSafeChannelGen[runningStatus](1)
		go func(s *SafeChannel[runningStatus]) {
			// 랜덤 딜레이 후 값을 전송
			time.Sleep(time.Duration(rand.Intn(5)) * time.Millisecond)
			s.Send(value)
			// 값 전송 후 채널을 닫음
			s.Close()
		}(sc)
		node.parentVertex[i] = sc
	}

	return node
}

func BenchmarkPreFlight(b *testing.B) {
	// 로그 기록이 벤치마크 결과에 영향을 주지 않도록 로그 출력 방지
	Log.SetOutput(io.Discard)
	ctx := context.Background()
	// 부모 SafeChannel 이 10개인 노드를 생성하며, 모두 Succeed 값을 보내도록 설정
	node := setupNode("benchmark_preFlight", 10, Succeed)

	// Warm-up: 벤치마크 루프 전에 한 번 실행하고, 이후 부모 채널을 재채움
	_ = preFlight(ctx, node)
	// 각 부모 SafeChannel 에 Succeed 값을 재삽입
	for j := 0; j < len(node.parentVertex); j++ {
		node.parentVertex[j].Send(Succeed)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = preFlight(ctx, node)
		// 각 반복 후 부모 채널을 다시 채워 다음 반복에 대비
		for j := 0; j < len(node.parentVertex); j++ {
			node.parentVertex[j].Send(Succeed)
		}
	}
}
