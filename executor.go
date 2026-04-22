package scheduler

import "context"

// JobExecutor executes a job's payload. Consumers implement this interface
// to handle specific job types (e.g., agent jobs, shell scripts, webhooks).
//
// Implementations must:
//   - Respect context cancellation
//   - Return a non-nil [JobResult] on success or an error on failure
//   - Handle their own timeout logic (job.Config.Timeout is informational)
//   - Distinguish retryable vs. deterministic errors if retry logic is needed
type JobExecutor interface {
	// Execute runs the job and returns the result or an error.
	// The context is cancelled if the scheduler stops or the parent context expires.
	Execute(ctx context.Context, job *Job) (*JobResult, error)
}
