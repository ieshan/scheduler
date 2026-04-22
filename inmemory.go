package scheduler

import (
	"context"
	"fmt"
	"sync"
)

// InMemoryJobStore is a thread-safe in-memory [JobStore] implementation for testing.
// It stores jobs in a map and provides full [JobStore] semantics.
//
// This is useful for unit tests and demos where persistence is not required.
// For production, implement [JobStore] with a database backend.
type InMemoryJobStore struct {
	mu   sync.RWMutex
	jobs map[string]*Job
}

// NewInMemoryJobStore creates a new empty in-memory job store.
func NewInMemoryJobStore() *InMemoryJobStore {
	return &InMemoryJobStore{jobs: make(map[string]*Job)}
}

func (s *InMemoryJobStore) List(_ context.Context) ([]Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		result = append(result, *j)
	}
	return result, nil
}

func (s *InMemoryJobStore) Get(_ context.Context, id string) (*Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.jobs[id]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrJobNotFound, id)
	}
	return new(*j), nil
}

func (s *InMemoryJobStore) Save(_ context.Context, job *Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = new(*job)
	return nil
}

func (s *InMemoryJobStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.jobs[id]; !ok {
		return fmt.Errorf("%w: %q", ErrJobNotFound, id)
	}
	delete(s.jobs, id)
	return nil
}

func (s *InMemoryJobStore) UpdateState(_ context.Context, id string, state JobState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	if !ok {
		return fmt.Errorf("%w: %q", ErrJobNotFound, id)
	}
	j.State = state
	return nil
}
