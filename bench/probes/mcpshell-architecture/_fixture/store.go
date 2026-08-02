package inventory

import "sync"

// Store is the only place inventory counts live.
type Store struct {
	mu     sync.Mutex
	counts map[string]int
}

func New() *Store { return &Store{counts: map[string]int{}} }

func (s *Store) Add(sku string, n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counts[sku] += n
}

func (s *Store) Get(sku string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.counts[sku]
}
