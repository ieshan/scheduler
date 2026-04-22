package scheduler

import (
	"context"
	"errors"
)

// Sentinel errors returned by [JobStore] implementations.
var (
	// ErrJobNotFound indicates the requested job ID does not exist in the store.
	ErrJobNotFound = errors.New("job not found")
)

// JobStore persists [Job] definitions and their runtime [JobState].
// Implementations provide in-memory or database-backed storage.
// The [Scheduler] calls these methods to load jobs, update state after execution,
// and persist changes.
type JobStore interface {
	// List returns all jobs in the store.
	List(ctx context.Context) ([]Job, error)

	// Get returns a single job by ID, or an error if not found.
	Get(ctx context.Context, id string) (*Job, error)

	// Save creates or overwrites a job in the store.
	// Returns an error if the job cannot be persisted.
	Save(ctx context.Context, job *Job) error

	// Delete removes a job from the store.
	// Returns an error if the job is not found.
	Delete(ctx context.Context, id string) error

	// UpdateState updates only the [JobState] fields of a job
	// (NextRun, LastRun, LastStatus, LastOutput, RunCount).
	// The scheduler calls this after each execution.
	// Returns an error if the job is not found.
	UpdateState(ctx context.Context, id string, state JobState) error
}
