package scheduler

import "time"

// Job is a scheduled task with a schedule, executor type, payload, and runtime state.
// A job's state (NextRun, LastRun, RunCount) is updated by the scheduler after each execution.
type Job struct {
	// ID is the unique identifier for this job.
	ID string `json:"id"`

	// Name is a human-readable name for the job.
	Name string `json:"name"`

	// Schedule is the parsed [Schedule] interface (Cron, Every, or At).
	// Not serialized directly; the schedule type and expression are stored in ScheduleType.
	Schedule Schedule `json:"-"` // serialized separately

	// ScheduleType is the schedule type name ("cron", "every", "at").
	// The scheduler uses this to reconstruct the Schedule interface on load.
	ScheduleType string `json:"schedule_type"`

	// ScheduleExpression is the raw schedule expression used for persistence.
	// For "cron": "0 9 * * *", for "every": "5m", for "at": RFC3339 timestamp.
	// This is used to reconstruct the Schedule interface when loading from storage.
	ScheduleExpression string `json:"schedule_expression"`

	// Payload is optional user-defined data passed to the executor.
	Payload any `json:"payload"`

	// Enabled indicates whether the job should be dispatched when it is due.
	// The scheduler skips disabled jobs during tick.
	Enabled bool `json:"enabled"`

	// DeleteAfterRun indicates this is a one-shot job that should be disabled after successful execution.
	DeleteAfterRun bool `json:"delete_after_run"`

	// ExecutorType identifies which executor in the [Scheduler] should run this job
	// (e.g., "agent", "shell", or a custom type).
	ExecutorType string `json:"executor_type"` // "agent", "shell", etc.

	// Config holds execution settings for this job (timeout, retries, backoff).
	Config JobConfig `json:"config"`

	// State holds runtime state: next scheduled run, last run time, status, output.
	// Updated by the scheduler after each execution.
	State JobState `json:"state"`

	// ChannelKey identifies the delivery target (e.g. "tg:123", "tui:local").
	// If set, the scheduler routes job results via [DeliveryService].
	ChannelKey string `json:"channel_key,omitempty"`

	// Prompt is the text sent to the agent executor.
	// Used by [AgentJobExecutor] in the agent module.
	Prompt string `json:"prompt,omitempty"`

	// Script is the shell command for [ShellJobExecutor].
	// Used by shell-based executors.
	Script string `json:"script,omitempty"`
}

// JobConfig holds per-job execution settings.
type JobConfig struct {
	// Timeout is the maximum duration a job is allowed to run.
	// If zero, no timeout is enforced.
	Timeout time.Duration `json:"timeout"`

	// MaxRetries is the maximum number of times a retryable error triggers a retry.
	// If zero, no retries are attempted (fail immediately on error).
	MaxRetries int `json:"max_retries"`

	// RetryBackoff is the base duration for jittered exponential backoff between retries.
	// Actual backoff: [RetryBackoff] * 2^attempt * (0.5 + random).
	RetryBackoff time.Duration `json:"retry_backoff"`
}

// JobState holds runtime state for a job, updated after each execution.
type JobState struct {
	// NextRun is the next scheduled time this job should execute.
	// Calculated by the scheduler based on the job's Schedule.
	NextRun time.Time `json:"next_run"`

	// LastRun is the most recent time this job was executed.
	LastRun time.Time `json:"last_run"`

	// LastStatus is the outcome of the most recent execution (success, failed, retrying, skipped).
	LastStatus JobStatus `json:"last_status"`

	// LastOutput is the output or error message from the most recent execution.
	LastOutput string `json:"last_output"`

	// RunCount is the number of times this job has been executed.
	RunCount int `json:"run_count"`
}
