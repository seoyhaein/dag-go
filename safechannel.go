package dag_go

import (
	"fmt"
	"sync"
)

// 제네릭 SafeChannel 타입. T는 채널 요소의 타입.
type SafeChannel[T any] struct {
	ch     chan T
	closed bool
	mu     sync.RWMutex
}

// NewSafeChannelGen 은 주어진 버퍼 크기로 새로운 제네릭 SafeChannel 을 생성한다.
func NewSafeChannelGen[T any](buffer int) *SafeChannel[T] {
	return &SafeChannel[T]{
		ch:     make(chan T, buffer),
		closed: false,
	}
}

// Send 는 SafeChannel 에 안전하게 값(T 타입)을 보내고, 성공 시 true 를 반환한다.
func (sc *SafeChannel[T]) Send(value T) bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	if sc.closed {
		return false
	}
	select {
	case sc.ch <- value:
		return true
	default:
		return false
	}
}

// Close 는 SafeChannel 의 내부 채널을 안전하게 닫는다.
func (sc *SafeChannel[T]) Close() (err error) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.closed {
		return fmt.Errorf("channel already closed")
	}

	// panic 이 발생하면 err 에 메시지를 저장한다.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic while closing channel: %v", r)
		}
	}()

	close(sc.ch)
	sc.closed = true
	return nil
}

// GetChannel 은 내부 채널을 반환한다.
func (sc *SafeChannel[T]) GetChannel() chan T {
	return sc.ch
}
