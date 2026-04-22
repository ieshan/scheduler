// Package scheduler provides a periodic task scheduler with zero external
// dependencies. It supports cron expressions, fixed intervals, and one-shot
// schedules, with configurable concurrency, jittered
// exponential backoff, and graceful shutdown.
//
// Key types:
//   - [Scheduler] — orchestrates job dispatch
//   - [Job] — a unit of scheduled work with its state and config
//   - [JobExecutor] — the interface callers implement to run jobs
//   - [JobStore] — persistence interface (in-memory or custom)
//
// See https://github.com/ieshan/scheduler for guides.
package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Config configures the [Scheduler].
type Config struct {
	// Store provides job persistence and query. Required.
	Store JobStore

	// Executors maps executor type names (e.g., "agent", "shell") to their [JobExecutor] implementations.
	// Required. The scheduler uses this map to dispatch jobs to their executors.
	Executors map[string]JobExecutor

	// MaxConcurrent limits how many jobs execute simultaneously.
	// Default: 5. Must be positive.
	MaxConcurrent int

	// PollInterval is the time between store polls.
	// Default: 30s. Must be positive.
	PollInterval time.Duration

	// Logger receives scheduler operational messages (job dispatch, errors, state updates).
	// Default: slog.Default().
	Logger *slog.Logger
}

// Scheduler runs jobs on their schedules. It periodically polls the job store,
// identifies due jobs (where NextRun <= now), and dispatches them to registered [JobExecutor] implementations
// with bounded concurrency and graceful shutdown.
//
// Create a scheduler with [New], start it with [Scheduler.Start], add jobs via the job store,
// and stop it with [Scheduler.Stop].
//
// Example:
//
//	store := scheduler.NewInMemoryJobStore()
//	cfg := scheduler.Config{
//		Store: store,
//		Executors: map[string]scheduler.JobExecutor{"shell": myShellExecutor},
//		MaxConcurrent: 5,
//		PollInterval: 30 * time.Second,
//	}
//	sched := scheduler.New(cfg)
//	go sched.Start(context.Background())
//	defer sched.Stop()
//
//	// Add a cron job to the store, then the scheduler will dispatch it...
type Scheduler struct {
	store     JobStore
	executors map[string]JobExecutor
	maxConc   int
	poll      time.Duration
	logger    *slog.Logger
	nowFunc   func() time.Time
	wake      chan struct{}
	stopOnce  sync.Once
	stopCh    chan struct{}
	mu        sync.Mutex
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	jobQueue  chan Job
	delivery  DeliveryService
}

// Option configures a [Scheduler] after construction.
// Used with [Scheduler.New].
type Option func(*Scheduler)

// WithNowFunc overrides the clock used by the scheduler.
// By default, [Scheduler] uses [time.Now]. This is primarily useful for testing
// where a fake clock eliminates timing-dependent sleeps and makes tests deterministic.
//
// Example (for tests):
//
//	sched := scheduler.New(cfg, scheduler.WithNowFunc(func() time.Time {
//		return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
//	}))
func WithNowFunc(f func() time.Time) Option {
	return func(s *Scheduler) {
		s.nowFunc = f
	}
}

// WithDelivery registers a [DeliveryService] that is called after each job completes.
// The service routes job results back to the originating channel (e.g., Telegram, TUI).
// If not set, results are discarded (but still updated in the store).
func WithDelivery(svc DeliveryService) Option {
	return func(s *Scheduler) { s.delivery = svc }
}

// New creates a new [Scheduler] from the given [Config].
// The scheduler is not started; call [Scheduler.Start] to begin dispatching jobs.
// Options are applied after construction and may override config defaults.
func New(cfg Config, opts ...Option) *Scheduler {
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 5
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 30 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	s := &Scheduler{
		store:     cfg.Store,
		executors: cfg.Executors,
		maxConc:   cfg.MaxConcurrent,
		poll:      cfg.PollInterval,
		logger:    cfg.Logger,
		nowFunc:   time.Now,
		wake:      make(chan struct{}, 1),
		stopCh:    make(chan struct{}),
		jobQueue:  make(chan Job, cfg.MaxConcurrent*2),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Start runs the scheduler's main loop.
// It blocks until [Scheduler.Stop] is called or ctx is cancelled.
// Start polls the job store at the configured [Config.PollInterval],
// identifies due jobs (NextRun <= now), and dispatches them to their [JobExecutor] implementations.
//
// Typically called in a goroutine:
//
//	go sched.Start(context.Background())
func (s *Scheduler) Start(ctx context.Context) {
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()

	// Start worker pool
	for i := 0; i < s.maxConc; i++ {
		s.wg.Add(1)
		go s.worker(ctx)
	}

	timer := time.NewTimer(0) // fire immediately on start
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			close(s.jobQueue) // Signal workers to exit
			return
		case <-s.stopCh:
			close(s.jobQueue) // Signal workers to exit
			return
		case <-s.wake:
			s.tick(ctx)
			timer.Reset(s.poll)
		case <-timer.C:
			s.tick(ctx)
			timer.Reset(s.poll)
		}
	}
}

// worker processes jobs from the job queue
func (s *Scheduler) worker(ctx context.Context) {
	defer s.wg.Done()
	for job := range s.jobQueue {
		s.executeJob(ctx, job)
	}
}

// executeJob runs a single job and handles state updates
func (s *Scheduler) executeJob(ctx context.Context, job Job) {
	now := s.nowFunc()
	executor, ok := s.executors[job.ExecutorType]
	if !ok {
		s.logger.Warn("scheduler: no executor for job", "job_id", job.ID, "executor_type", job.ExecutorType)
		return
	}

	result, execErr := executor.Execute(ctx, &job)

	// Update state
	state := job.State
	state.LastRun = now
	state.RunCount++
	if execErr != nil {
		state.LastStatus = StatusFailed
		state.LastOutput = execErr.Error()
	} else if result != nil {
		state.LastStatus = result.Status
		state.LastOutput = result.Output
	}
	// Calculate next run
	if job.Schedule != nil {
		state.NextRun = job.Schedule.NextTick(now)
	}

	if err := s.store.UpdateState(ctx, job.ID, state); err != nil {
		s.logger.Warn("scheduler: failed to update job state",
			"job_id", job.ID, "error", err)
	}

	// Deliver result to originating channel if configured
	if s.delivery != nil && job.ChannelKey != "" {
		dr := &JobResult{ChannelKey: job.ChannelKey}
		if execErr != nil {
			dr.Status = StatusFailed
			dr.Error = execErr.Error()
		} else if result != nil {
			dr.Status = result.Status
			dr.Output = result.Output
			dr.Error = result.Error
			dr.Duration = result.Duration
			dr.Silent = result.Silent
		}
		if err := s.delivery.Deliver(ctx, dr); err != nil {
			s.logger.Warn("scheduler: delivery failed",
				"job_id", job.ID, "error", err)
		}
	}

	// One-shot: disable after run
	if job.DeleteAfterRun {
		disabledJob := job
		disabledJob.Enabled = false
		if err := s.store.Save(ctx, &disabledJob); err != nil {
			s.logger.Warn("scheduler: failed to disable one-shot job",
				"job_id", job.ID, "error", err)
		}
	}
}

// Stop gracefully stops the scheduler and waits for all in-flight jobs to complete.
// The context passed to [Scheduler.Start] is cancelled to signal executors to stop,
// and Stop blocks until all goroutines have exited.
//
// After Stop returns, no further jobs will be dispatched.
// Stop is safe to call multiple times (idempotent).
func (s *Scheduler) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
		s.mu.Lock()
		cancel := s.cancel
		s.mu.Unlock()
		if cancel != nil {
			cancel()
		}
	})
	s.wg.Wait()
}

// Wake signals the scheduler to re-evaluate jobs immediately,
// without waiting for the next [Config.PollInterval].
// This is useful when a job is added to the store and you want it to run ASAP.
// Wake is safe to call concurrently and does not block.
func (s *Scheduler) Wake() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	jobs, err := s.store.List(ctx)
	if err != nil {
		s.logger.Warn("scheduler: failed to list jobs", "error", err)
		return
	}

	now := s.nowFunc()

	for i := range jobs {
		job := jobs[i]
		if !job.Enabled {
			continue
		}
		if job.State.NextRun.After(now) {
			continue
		}

		// Check if executor exists before queuing
		if _, ok := s.executors[job.ExecutorType]; !ok {
			continue
		}

		// Send to job queue (non-blocking with select to avoid deadlock on shutdown)
		select {
		case s.jobQueue <- job:
		case <-ctx.Done():
			return
		}
	}
}
