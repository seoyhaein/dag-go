package dag_go

import (
	"fmt"
	"sync"
)

type SafeChannel[T any] struct {
	ch     chan T
	closed bool
	mu     sync.RWMutex
}

func NewSafeChannelGen[T any](buffer int) *SafeChannel[T] {
	return &SafeChannel[T]{
		ch:     make(chan T, buffer),
		closed: false,
	}
}

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

func (sc *SafeChannel[T]) GetChannel() chan T {
	return sc.ch
}
