package scheduler

import "sync"

type Registry interface {
	Get(spawnID string) (*Actor, bool)
	Put(spawnID string, a *Actor)
	Delete(spawnID string)
}

type InMemoryRegistry struct {
	mu   sync.RWMutex
	data map[string]*Actor
}

func NewInMemoryRegistry() *InMemoryRegistry {
	return &InMemoryRegistry{data: make(map[string]*Actor)}
}
func (r *InMemoryRegistry) Get(spawnID string) (*Actor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.data[spawnID]
	return a, ok
}
func (r *InMemoryRegistry) Put(spawnID string, a *Actor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data[spawnID] = a
}
func (r *InMemoryRegistry) Delete(spawnID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.data, spawnID)
}
