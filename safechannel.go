package dag_go

import (
	"context"
	"fmt"
	"sync"
)

// SafeChannel is a generic, concurrency-safe channel wrapper that prevents
// double-close panics and provides non-blocking send semantics.
type SafeChannel[T any] struct {
	ch     chan T
	closed bool
	mu     sync.RWMutex
}

// NewSafeChannelGen creates a new SafeChannel with the given buffer size.
func NewSafeChannelGen[T any](buffer int) *SafeChannel[T] {
	return &SafeChannel[T]{
		ch:     make(chan T, buffer),
		closed: false,
	}
}

// Send attempts to deliver value to the channel. Returns false if the channel
// is already closed or the buffer is full.
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

// SendBlocking blocks until value is delivered to the channel, the context is
// cancelled, or the channel is closed.  Returns true when the value was sent;
// false when the channel was already closed or ctx.Done fired first.
//
// Unlike Send, SendBlocking never silently drops a value when the buffer is
// full — it waits for a consumer to make room.  Use this for signals where
// loss would leave a downstream goroutine waiting forever (e.g. edge vertex
// channels between nodes).
//
// The read lock is held for the duration of the blocking select so that a
// concurrent Close cannot race with the send.  In practice the lock is
// released promptly because either the consumer unblocks the send or ctx is
// cancelled.
func (sc *SafeChannel[T]) SendBlocking(ctx context.Context, value T) bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	if sc.closed {
		return false
	}
	select {
	case sc.ch <- value:
		return true
	case <-ctx.Done():
		return false
	}
}

// Close closes the underlying channel exactly once. Returns an error if the
// channel is already closed or a panic occurs during close.
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

// GetChannel returns the underlying channel for range/select operations.
func (sc *SafeChannel[T]) GetChannel() chan T {
	return sc.ch
}
