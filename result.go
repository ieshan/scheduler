package scheduler

import "time"

// JobStatus represents the outcome of a job execution.
type JobStatus string

const (
	// StatusSuccess indicates the job completed successfully.
	StatusSuccess JobStatus = "success"

	// StatusFailed indicates the job encountered a non-retryable error.
	StatusFailed JobStatus = "failed"

	// StatusRetrying indicates the job will be retried (set by executor or scheduler retry logic).
	StatusRetrying JobStatus = "retrying"

	// StatusSkipped indicates the job was intentionally skipped (executor discretion).
	StatusSkipped JobStatus = "skipped"
)

// JobResult is the outcome of a single job execution.
// Returned by [JobExecutor.Execute].
type JobResult struct {
	// Status is the outcome (success, failed, retrying, skipped).
	Status JobStatus `json:"status"`

	// Output is the execution output (stdout, response body, etc.).
	Output string `json:"output"`

	// Error is the error message if Status is failed or retrying.
	// Empty if Status is success or skipped.
	Error string `json:"error,omitempty"`

	// Duration is the time taken to execute the job.
	Duration time.Duration `json:"duration"`

	// ChannelKey identifies the delivery target (e.g., "tg:123").
	// Set by the executor if the job result should be routed back to a channel.
	ChannelKey string `json:"channel_key,omitempty"` // delivery target

	// Silent suppresses delivery when true.
	// If set, the scheduler skips calling [DeliveryService.Deliver].
	Silent bool `json:"silent,omitempty"`
}
