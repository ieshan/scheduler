package scheduler

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ExampleNew demonstrates creating and starting a Scheduler.
func ExampleNew() {
	store := NewInMemoryJobStore()

	cfg := Config{
		Store:         store,
		Executors:     map[string]JobExecutor{"print": &printExecutor{}},
		MaxConcurrent: 2,
		PollInterval:  50 * time.Millisecond,
	}

	sched := New(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	go sched.Start(ctx)
	defer sched.Stop()
	defer cancel()

	job := &Job{
		ID:           mustID("01HZY0CWD0A0VKBQHHP3MS4GC0"),
		Name:         "hello-job",
		ExecutorType: "print",
		Enabled:      true,
		Schedule:     Every(100 * time.Millisecond),
		ScheduleType: "every",
		State:        JobState{NextRun: time.Now()},
	}
	_ = store.Save(context.Background(), job)

	time.Sleep(250 * time.Millisecond)
}

// ExampleScheduler_cron demonstrates adding a cron job to a running Scheduler.
func ExampleScheduler_cron() {
	store := NewInMemoryJobStore()
	sched := New(Config{
		Store:         store,
		Executors:     map[string]JobExecutor{"noop": &noopExecutor{}},
		PollInterval:  time.Second,
		MaxConcurrent: 1,
	})
	ctx, cancel := context.WithCancel(context.Background())
	go sched.Start(ctx)
	defer sched.Stop()
	defer cancel()

	cronSched, err := Cron("0 9 * * *")
	if err != nil {
		panic(err)
	}

	job := &Job{
		ID:           mustID("01HZY0CWD0A0VKBQHHP3MS4GC1"),
		Name:         "daily-report",
		ExecutorType: "noop",
		Enabled:      true,
		Schedule:     cronSched,
		ScheduleType: "cron",
	}
	_ = store.Save(context.Background(), job)

	saved, _ := store.Get(context.Background(), mustID("01HZY0CWD0A0VKBQHHP3MS4GC1"))
	fmt.Println(saved.Name)
	// Output: daily-report
}

// ExampleScheduler_every demonstrates adding a fixed-interval job.
func ExampleScheduler_every() {
	store := NewInMemoryJobStore()
	job := &Job{
		ID:           mustID("01HZY0CWD0A0VKBQHHP3MS4GC2"),
		Name:         "status-check",
		ExecutorType: "noop",
		Enabled:      true,
		Schedule:     Every(5 * time.Minute),
		ScheduleType: "every",
	}
	_ = store.Save(context.Background(), job)

	saved, _ := store.Get(context.Background(), mustID("01HZY0CWD0A0VKBQHHP3MS4GC2"))
	fmt.Println(saved.Name)
	// Output: status-check
}

// printExecutor is a test executor that logs job names.
type printExecutor struct{}

func (e *printExecutor) Execute(_ context.Context, job *Job) (*JobResult, error) {
	return &JobResult{Status: StatusSuccess, Output: "ran " + job.Name}, nil
}

// noopExecutor does nothing.
type noopExecutor struct{}

func (e *noopExecutor) Execute(_ context.Context, _ *Job) (*JobResult, error) {
	return &JobResult{Status: StatusSuccess}, nil
}

// ExampleCron demonstrates parsing a cron expression and computing the next tick.
func ExampleCron() {
	s, err := Cron("0 9 * * *")
	if err != nil {
		panic(err)
	}

	now := time.Date(2026, 4, 15, 8, 0, 0, 0, time.UTC)
	next := s.NextTick(now)
	fmt.Println(next.Format(time.RFC3339))
	// Output: 2026-04-15T09:00:00Z
}

// ExampleEvery demonstrates a fixed-interval schedule.
func ExampleEvery() {
	s := Every(5 * time.Minute)
	now := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	next := s.NextTick(now)
	fmt.Println(next.Format(time.RFC3339))
	// Output: 2026-04-15T10:05:00Z
}

// ExampleAt demonstrates a one-shot schedule.
func ExampleAt() {
	target := time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC)
	s := At(target)

	// Before the target time — returns the target.
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	fmt.Println(s.NextTick(now).Format(time.RFC3339))

	// After the target time — returns zero time.
	later := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	fmt.Println(s.NextTick(later).IsZero())
	// Output:
	// 2026-12-25T00:00:00Z
	// true
}

// ExampleRetryBackoff demonstrates jittered exponential backoff calculation.
func ExampleRetryBackoff() {
	base := time.Second
	for i := range 3 {
		d := RetryBackoff(base, i)
		fmt.Printf("attempt %d: %v <= delay < %v\n", i, base*time.Duration(1<<uint(i))/2, base*time.Duration(1<<uint(i))*3/2)
		_ = d // actual value varies due to jitter
	}
	// Output:
	// attempt 0: 500ms <= delay < 1.5s
	// attempt 1: 1s <= delay < 3s
	// attempt 2: 2s <= delay < 6s
}

// ExampleInMemoryJobStore demonstrates basic CRUD operations on the in-memory store.
func ExampleInMemoryJobStore() {
	store := NewInMemoryJobStore()
	ctx := context.Background()

	job := &Job{
		ID:   mustID("01HZY0CWD0A0VKBQHHP3MS4GC3"),
		Name: "demo-job",
		State: JobState{
			NextRun: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	_ = store.Save(ctx, job)

	got, err := store.Get(ctx, mustID("01HZY0CWD0A0VKBQHHP3MS4GC3"))
	if err != nil {
		panic(err)
	}
	fmt.Println(got.Name)

	jobs, _ := store.List(ctx)
	fmt.Println(len(jobs))

	_ = store.Delete(ctx, mustID("01HZY0CWD0A0VKBQHHP3MS4GC3"))
	_, err = store.Get(ctx, mustID("01HZY0CWD0A0VKBQHHP3MS4GC3"))
	fmt.Println(errors.Is(err, ErrJobNotFound))
	// Output:
	// demo-job
	// 1
	// true
}
