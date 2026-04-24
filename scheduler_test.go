package scheduler

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type countExecutor struct {
	count atomic.Int64
	done  chan struct{} // closed after first execution
	once  sync.Once
}

func newCountExecutor() *countExecutor {
	return &countExecutor{done: make(chan struct{})}
}

func (e *countExecutor) Execute(_ context.Context, _ *Job) (*JobResult, error) {
	e.count.Add(1)
	e.once.Do(func() { close(e.done) })
	return &JobResult{Status: StatusSuccess, Output: "ok"}, nil
}

func TestScheduler_ExecutesJobOnTime(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	store := NewInMemoryJobStore()
	exec := newCountExecutor()

	store.Save(ctx, &Job{
		ID:           "j1",
		Name:         "test",
		Enabled:      true,
		ExecutorType: "test",
		Schedule:     Every(100 * time.Millisecond),
		State: JobState{
			NextRun: time.Now(),
		},
	})

	s := New(Config{
		Store:         store,
		MaxConcurrent: 2,
		PollInterval:  50 * time.Millisecond,
		Executors: map[string]JobExecutor{
			"test": exec,
		},
	})

	go s.Start(ctx)

	// Wait for first execution via channel instead of time.Sleep.
	select {
	case <-exec.done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for job execution")
	}
	s.Stop()

	if exec.count.Load() < 1 {
		t.Errorf("expected at least 1 execution, got %d", exec.count.Load())
	}
}

func TestScheduler_DisabledJobsSkipped(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	store := NewInMemoryJobStore()
	exec := newCountExecutor()

	store.Save(ctx, &Job{
		ID:           "disabled",
		Name:         "skip me",
		Enabled:      false,
		ExecutorType: "test",
		Schedule:     Every(50 * time.Millisecond),
		State: JobState{
			NextRun: time.Now(),
		},
	})

	// Also add an enabled job so we can wait for at least one tick.
	enabledExec := newCountExecutor()
	store.Save(ctx, &Job{
		ID:           "enabled",
		Name:         "run me",
		Enabled:      true,
		ExecutorType: "enabled",
		Schedule:     Every(50 * time.Millisecond),
		State:        JobState{NextRun: time.Now()},
	})

	s := New(Config{
		Store:        store,
		PollInterval: 50 * time.Millisecond,
		Executors: map[string]JobExecutor{
			"test":    exec,
			"enabled": enabledExec,
		},
	})

	go s.Start(ctx)

	// Wait for the enabled job to run (proves the tick happened).
	select {
	case <-enabledExec.done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for enabled job")
	}
	s.Stop()

	if exec.count.Load() != 0 {
		t.Errorf("disabled job should not execute, got %d runs", exec.count.Load())
	}
}

func TestScheduler_GracefulStop(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	store := NewInMemoryJobStore()
	s := New(Config{
		Store:        store,
		PollInterval: 50 * time.Millisecond,
		Executors:    map[string]JobExecutor{},
	})

	started := make(chan struct{})
	go func() {
		close(started)
		s.Start(ctx)
	}()
	<-started // ensure Start is running
	s.Stop()  // should not panic or hang
}

func TestScheduler_WakeOnChange(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	store := NewInMemoryJobStore()
	exec := newCountExecutor()

	s := New(Config{
		Store:        store,
		PollInterval: 10 * time.Second, // very long poll — should wake immediately
		Executors:    map[string]JobExecutor{"test": exec},
	})

	started := make(chan struct{})
	go func() {
		close(started)
		s.Start(ctx)
	}()
	<-started

	// Add a job that's due now and wake the scheduler.
	store.Save(ctx, &Job{
		ID: "wake", Name: "wake", Enabled: true, ExecutorType: "test",
		Schedule: Every(50 * time.Millisecond),
		State:    JobState{NextRun: time.Now()},
	})
	s.Wake()

	select {
	case <-exec.done:
	case <-ctx.Done():
		t.Fatal("wake should trigger execution")
	}
	s.Stop()

	if exec.count.Load() < 1 {
		t.Errorf("wake should trigger execution, got %d", exec.count.Load())
	}
}

func TestScheduler_StopWaitsForInFlight(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	store := NewInMemoryJobStore()

	// Use an executor that does NOT respond to ctx.Done — it only unblocks
	// via the explicit channel. This proves Stop's wg.Wait actually waits.
	unblock := make(chan struct{})
	started := make(chan struct{})
	stubExec := executorFunc(func(_ context.Context, _ *Job) (*JobResult, error) {
		close(started)
		<-unblock // deliberately ignores context
		return &JobResult{Status: StatusSuccess}, nil
	})

	store.Save(ctx, &Job{
		ID: "blocking", Name: "blocking", Enabled: true, ExecutorType: "block",
		Schedule: Every(time.Hour),
		State:    JobState{NextRun: time.Now()},
	})

	s := New(Config{
		Store:         store,
		PollInterval:  50 * time.Millisecond,
		MaxConcurrent: 2,
		Executors:     map[string]JobExecutor{"block": stubExec},
	})

	go s.Start(ctx)

	// Wait for executor to start.
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for executor to start")
	}

	// Stop should block until we unblock the executor.
	stopDone := make(chan struct{})
	go func() {
		s.Stop()
		close(stopDone)
	}()

	// Verify Stop hasn't returned yet (executor is still blocked).
	select {
	case <-stopDone:
		t.Fatal("Stop returned before in-flight job completed")
	case <-time.After(100 * time.Millisecond):
		// expected — Stop is still waiting
	}

	// Unblock the executor; Stop should now return.
	close(unblock)
	select {
	case <-stopDone:
		// success
	case <-time.After(3 * time.Second):
		t.Fatal("Stop did not return after in-flight job completed")
	}
}

func TestScheduler_StopCancelsContext(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	store := NewInMemoryJobStore()
	ctxErr := make(chan error, 1)
	started := make(chan struct{})

	store.Save(ctx, &Job{
		ID: "ctx-cancel", Name: "ctx-cancel", Enabled: true, ExecutorType: "block",
		Schedule: Every(time.Hour),
		State:    JobState{NextRun: time.Now()},
	})

	exec := executorFunc(func(ctx context.Context, _ *Job) (*JobResult, error) {
		close(started)
		<-ctx.Done()
		ctxErr <- ctx.Err()
		return nil, ctx.Err()
	})

	s := New(Config{
		Store:         store,
		PollInterval:  50 * time.Millisecond,
		MaxConcurrent: 2,
		Executors:     map[string]JobExecutor{"block": exec},
	})

	go s.Start(ctx)

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for executor to start")
	}

	s.Stop()

	select {
	case err := <-ctxErr:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("executor did not observe context cancellation")
	}
}

// executorFunc adapts a function to the JobExecutor interface.
type executorFunc func(context.Context, *Job) (*JobResult, error)

func (f executorFunc) Execute(ctx context.Context, j *Job) (*JobResult, error) {
	return f(ctx, j)
}

// fakeClock allows tests to control the scheduler's notion of "now".
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// TestScheduler_WithNowFunc verifies that the fake clock injection controls
// which jobs the scheduler treats as due. A job whose NextRun is in the future
// relative to the fake clock must not fire; advancing the clock past NextRun
// must cause it to fire on the next tick.
func TestScheduler_WithNowFunc(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	base := time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: base}

	store := NewInMemoryJobStore()

	// futureExec signals when the future job runs.
	futureExec := newCountExecutor()
	// immediateExec signals when the immediately-due job runs (used as a tick probe).
	immediateExec := newCountExecutor()

	// job-A: due in the future relative to fake clock — must NOT run initially.
	store.Save(ctx, &Job{
		ID: "future", Name: "future", Enabled: true, ExecutorType: "future",
		Schedule: Every(time.Hour),
		State:    JobState{NextRun: base.Add(time.Minute)},
	})
	// job-B: already past due relative to fake clock — must run immediately.
	store.Save(ctx, &Job{
		ID: "immediate", Name: "immediate", Enabled: true, ExecutorType: "immediate",
		Schedule: Every(time.Hour),
		State:    JobState{NextRun: base.Add(-time.Second)},
	})

	s := New(
		Config{
			Store:         store,
			MaxConcurrent: 2,
			PollInterval:  50 * time.Millisecond,
			Executors: map[string]JobExecutor{
				"future":    futureExec,
				"immediate": immediateExec,
			},
		},
		WithNowFunc(clock.Now), // inject fake clock
	)

	go s.Start(ctx)

	// Wait for the immediate job to run — this proves a tick fired at time base.
	select {
	case <-immediateExec.done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for immediate job")
	}

	// The future job must not have run yet.
	if futureExec.count.Load() != 0 {
		t.Fatalf("future job ran before clock advanced: count = %d", futureExec.count.Load())
	}

	// Advance the fake clock past job-A's NextRun and trigger a tick.
	clock.Advance(2 * time.Minute)
	s.Wake()

	// Now the future job must fire.
	select {
	case <-futureExec.done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for future job after clock advance")
	}
	s.Stop()

	if futureExec.count.Load() < 1 {
		t.Errorf("future job count = %d, want ≥ 1", futureExec.count.Load())
	}
}

func TestScheduler_SemaphoreLimitsConcurrency(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	store := NewInMemoryJobStore()
	const maxConc = 2
	const numJobs = 5

	var running atomic.Int64
	var maxSeen atomic.Int64
	allStarted := make(chan struct{})
	var startCount atomic.Int64

	exec := executorFunc(func(ctx context.Context, _ *Job) (*JobResult, error) {
		cur := running.Add(1)
		defer running.Add(-1)
		// Track max concurrency seen.
		for {
			old := maxSeen.Load()
			if cur <= old || maxSeen.CompareAndSwap(old, cur) {
				break
			}
		}
		if startCount.Add(1) == numJobs {
			close(allStarted)
		}
		// Hold the slot briefly.
		select {
		case <-time.After(100 * time.Millisecond):
		case <-ctx.Done():
		}
		return &JobResult{Status: StatusSuccess}, nil
	})

	for i := range numJobs {
		store.Save(ctx, &Job{
			ID: fmt.Sprintf("j%d", i), Name: fmt.Sprintf("j%d", i),
			Enabled: true, ExecutorType: "test",
			Schedule: Every(time.Hour),
			State:    JobState{NextRun: time.Now()},
		})
	}

	s := New(Config{
		Store:         store,
		PollInterval:  50 * time.Millisecond,
		MaxConcurrent: maxConc,
		Executors:     map[string]JobExecutor{"test": exec},
	})

	go s.Start(ctx)

	select {
	case <-allStarted:
	case <-ctx.Done():
		t.Fatal("timed out waiting for all jobs to start")
	}
	s.Stop()

	if max := maxSeen.Load(); max > int64(maxConc) {
		t.Errorf("max concurrent = %d, want <= %d", max, maxConc)
	}
}

func TestScheduler_DefaultConfig(t *testing.T) {
	t.Parallel()
	s := New(Config{
		Store:     NewInMemoryJobStore(),
		Executors: map[string]JobExecutor{},
	})
	if s.maxConc != 5 {
		t.Errorf("default MaxConcurrent = %d, want 5", s.maxConc)
	}
	if s.poll != 30*time.Second {
		t.Errorf("default PollInterval = %v, want 30s", s.poll)
	}
	if s.logger == nil {
		t.Error("default Logger should not be nil")
	}
	if s.nowFunc == nil {
		t.Error("default nowFunc should not be nil")
	}
}

func TestScheduler_DeleteAfterRun(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	store := NewInMemoryJobStore()
	exec := newCountExecutor()

	store.Save(ctx, &Job{
		ID:             "one-shot",
		Name:           "one-shot",
		Enabled:        true,
		ExecutorType:   "test",
		DeleteAfterRun: true,
		Schedule:       Every(time.Hour),
		State:          JobState{NextRun: time.Now()},
	})

	s := New(Config{
		Store:         store,
		PollInterval:  50 * time.Millisecond,
		MaxConcurrent: 1,
		Executors:     map[string]JobExecutor{"test": exec},
	})

	go s.Start(ctx)

	select {
	case <-exec.done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for one-shot job execution")
	}
	s.Stop()

	job, err := store.Get(ctx, "one-shot")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if job.Enabled {
		t.Error("one-shot job should be disabled after execution")
	}
}

func TestScheduler_StopIdempotent(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	store := NewInMemoryJobStore()
	s := New(Config{
		Store:        store,
		PollInterval: 50 * time.Millisecond,
		Executors:    map[string]JobExecutor{},
	})
	started := make(chan struct{})
	go func() {
		close(started)
		s.Start(ctx)
	}()
	<-started
	s.Stop()
	s.Stop() // should not panic or hang
}

func TestScheduler_SuccessfulJobUpdatesState(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	store := NewInMemoryJobStore()
	exec := newCountExecutor()

	store.Save(ctx, &Job{
		ID:           "state-check",
		Name:         "state-check",
		Enabled:      true,
		ExecutorType: "test",
		Schedule:     Every(time.Hour),
		State:        JobState{NextRun: time.Now()},
	})

	s := New(Config{
		Store:         store,
		PollInterval:  50 * time.Millisecond,
		MaxConcurrent: 1,
		Executors:     map[string]JobExecutor{"test": exec},
	})

	go s.Start(ctx)

	select {
	case <-exec.done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for job execution")
	}
	s.Stop()

	job, err := store.Get(ctx, "state-check")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if job.State.LastStatus != StatusSuccess {
		t.Errorf("LastStatus = %q, want success", job.State.LastStatus)
	}
	if job.State.RunCount != 1 {
		t.Errorf("RunCount = %d, want 1", job.State.RunCount)
	}
	if job.State.LastRun.IsZero() {
		t.Error("LastRun should not be zero after execution")
	}
	if job.State.NextRun.IsZero() {
		t.Error("NextRun should be calculated after execution")
	}
	if job.State.LastOutput != "ok" {
		t.Errorf("LastOutput = %q, want %q", job.State.LastOutput, "ok")
	}
}

func TestScheduler_FailedJobUpdatesState(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	store := NewInMemoryJobStore()
	errExec := executorFunc(func(_ context.Context, _ *Job) (*JobResult, error) {
		return nil, fmt.Errorf("boom")
	})

	store.Save(ctx, &Job{
		ID: "fail", Name: "fail", Enabled: true, ExecutorType: "fail",
		Schedule: Every(time.Hour),
		State:    JobState{NextRun: time.Now()},
	})

	s := New(Config{
		Store:         store,
		PollInterval:  50 * time.Millisecond,
		MaxConcurrent: 1,
		Executors:     map[string]JobExecutor{"fail": errExec},
	})

	go s.Start(ctx)

	// Poll until the job state is updated.
	deadline := time.After(3 * time.Second)
	for {
		job, err := store.Get(ctx, "fail")
		if err == nil && job.State.LastStatus == StatusFailed {
			if job.State.LastOutput != "boom" {
				t.Errorf("LastOutput = %q, want %q", job.State.LastOutput, "boom")
			}
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for job state to be updated to failed")
		case <-time.After(20 * time.Millisecond):
		}
	}
	s.Stop()
}

// TestScheduler_MissingExecutor_SkipsJob verifies that jobs with unknown executor types
// are skipped (not executed) during the tick loop.
func TestScheduler_MissingExecutor_SkipsJob(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	store := NewInMemoryJobStore()
	exec := newCountExecutor()

	// Job with unknown executor type - should be skipped
	store.Save(ctx, &Job{
		ID: "unknown-exec", Name: "unknown-exec", Enabled: true, ExecutorType: "nonexistent",
		Schedule: Every(50 * time.Millisecond),
		State:    JobState{NextRun: time.Now()},
	})

	// Job with known executor type - should run
	store.Save(ctx, &Job{
		ID: "known", Name: "known", Enabled: true, ExecutorType: "test",
		Schedule: Every(50 * time.Millisecond),
		State:    JobState{NextRun: time.Now()},
	})

	s := New(Config{
		Store:        store,
		PollInterval: 50 * time.Millisecond,
		Executors:    map[string]JobExecutor{"test": exec},
	})

	go s.Start(ctx)

	// Wait for the known job to execute
	select {
	case <-exec.done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for known job")
	}
	s.Stop()

	// Verify the unknown-exec job's state was never updated (no RunCount increment)
	unknownJob, err := store.Get(ctx, "unknown-exec")
	if err != nil {
		t.Fatalf("Get unknown-exec: %v", err)
	}
	if unknownJob.State.RunCount != 0 {
		t.Errorf("unknown executor job RunCount = %d, want 0", unknownJob.State.RunCount)
	}
}
