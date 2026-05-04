package scheduler

import (
	"context"
	"fmt"
	"sync"

	"github.com/ieshan/idx"
)

// InMemoryJobStore is a thread-safe in-memory [JobStore] implementation for testing.
// It stores jobs in a map and provides full [JobStore] semantics.
//
// This is useful for unit tests and demos where persistence is not required.
// For production, implement [JobStore] with a database backend.
type InMemoryJobStore struct {
	mu   sync.RWMutex
	jobs map[idx.ID]*Job
}

// NewInMemoryJobStore creates a new empty in-memory job store.
func NewInMemoryJobStore() *InMemoryJobStore {
	return &InMemoryJobStore{jobs: make(map[idx.ID]*Job)}
}

// List returns all jobs in the store.
func (s *InMemoryJobStore) List(_ context.Context) ([]Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		result = append(result, *j)
	}
	return result, nil
}

// Get returns a single job by ID, or [ErrJobNotFound] if not found.
func (s *InMemoryJobStore) Get(_ context.Context, id idx.ID) (*Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.jobs[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrJobNotFound, id.String())
	}
	return new(*j), nil
}

// Save creates or overwrites a job in the store.
func (s *InMemoryJobStore) Save(_ context.Context, job *Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = new(*job)
	return nil
}

// Delete removes a job from the store.
// Returns [ErrJobNotFound] if the job does not exist.
func (s *InMemoryJobStore) Delete(_ context.Context, id idx.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.jobs[id]; !ok {
		return fmt.Errorf("%w: %s", ErrJobNotFound, id.String())
	}
	delete(s.jobs, id)
	return nil
}

// UpdateState updates only the [JobState] fields of a job.
// Returns [ErrJobNotFound] if the job does not exist.
func (s *InMemoryJobStore) UpdateState(_ context.Context, id idx.ID, state JobState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrJobNotFound, id.String())
	}
	j.State = state
	return nil
}
