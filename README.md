# scheduler

[![Go Reference](https://pkg.go.dev/badge/github.com/ieshan/scheduler.svg)](https://pkg.go.dev/github.com/ieshan/scheduler)
[![Go Report Card](https://goreportcard.com/badge/github.com/ieshan/scheduler)](https://goreportcard.com/report/github.com/ieshan/scheduler)

A lightweight, minimal-dependency periodic task scheduler for Go. It supports cron expressions, fixed intervals, and one-shot schedules with configurable concurrency, jittered exponential backoff, and graceful shutdown.

## Features

- **Minimal external dependencies** — only [github.com/ieshan/idx](https://github.com/ieshan/idx) for ID generation.
- **Multiple schedule types** — cron expressions (`0 9 * * *`), fixed intervals (`Every(5*time.Minute)`), and one-shot (`At(time.Now())`).
- **Bounded concurrency** — configure `MaxConcurrent` to limit simultaneous job execution.
- **Graceful shutdown** — `Stop` waits for in-flight jobs to finish and cancels their contexts.
- **Result delivery** — pluggable `DeliveryService` to route job output back to channels (e.g., Telegram, TUI).
- **Jittered backoff** — built-in `RetryBackoff` to prevent thundering-herd retries.
- **Test-friendly** — inject fake clocks and in-memory stores for deterministic tests.

## Installation

```bash
go get github.com/ieshan/scheduler
```

Requires **Go 1.26** or later.

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/ieshan/scheduler"
)

type printExecutor struct{}

func (e *printExecutor) Execute(_ context.Context, job *scheduler.Job) (*scheduler.JobResult, error) {
    return &scheduler.JobResult{
        Status: scheduler.StatusSuccess,
        Output: "ran " + job.Name,
    }, nil
}

func main() {
    store := scheduler.NewInMemoryJobStore()

    sched := scheduler.New(scheduler.Config{
        Store:     store,
        Executors: map[string]scheduler.JobExecutor{"print": &printExecutor{}},
    })

    go sched.Start(context.Background())
    defer sched.Stop()

    // Add a job that runs every minute.
    _ = store.Save(context.Background(), &scheduler.Job{
        ID:           "greet",
        Name:         "greeting",
        ExecutorType: "print",
        Enabled:      true,
        Schedule:     scheduler.Every(time.Minute),
        ScheduleType: "every",
        State:        scheduler.JobState{NextRun: time.Now()},
    })

    time.Sleep(2 * time.Minute) // let it fire twice
}
```

## Core Concepts

### Scheduler

The [`Scheduler`](https://pkg.go.dev/github.com/ieshan/scheduler#Scheduler) is the central orchestrator. It polls a [`JobStore`](https://pkg.go.dev/github.com/ieshan/scheduler#JobStore) at a configurable interval, identifies jobs whose `NextRun` is in the past, and dispatches them to the appropriate [`JobExecutor`](https://pkg.go.dev/github.com/ieshan/scheduler#JobExecutor).

```go
sched := scheduler.New(scheduler.Config{
    Store:          store,
    Executors:      executors,
    MaxConcurrent:  5,                // default
    PollInterval:   30 * time.Second, // default
    Logger:         slog.Default(),
})

go sched.Start(ctx)
defer sched.Stop()
```

Call [`Wake`](https://pkg.go.dev/github.com/ieshan/scheduler#Scheduler.Wake) to force an immediate poll when you add or modify a job:

```go
store.Save(ctx, newJob)
sched.Wake()
```

### Job

A [`Job`](https://pkg.go.dev/github.com/ieshan/scheduler#Job) defines what to run, when to run it, and how to deliver the result.

| Field | Description |
|-------|-------------|
| `ID` | Unique identifier. |
| `Name` | Human-readable name. |
| `Schedule` | A [`Schedule`](https://pkg.go.dev/github.com/ieshan/scheduler#Schedule) implementation (cron, every, at). |
| `ScheduleType` | `"cron"`, `"every"`, or `"at"` — used for persistence reconstruction. |
| `ExecutorType` | Key into the scheduler's executor map (e.g., `"shell"`, `"agent"`). |
| `Enabled` | Whether the scheduler should dispatch this job. |
| `DeleteAfterRun` | Disable the job after one execution (useful for one-shots). |
| `Config` | Per-job settings: timeout, max retries, backoff. |
| `State` | Runtime state: `NextRun`, `LastRun`, `LastStatus`, `RunCount`. |
| `ChannelKey` | Delivery target, e.g., `"tg:123"` for Telegram user 123. |
| `Payload` | Arbitrary user data passed to the executor. |

### Schedules

Three built-in schedule types are provided:

- [`Cron("0 9 * * *")`](https://pkg.go.dev/github.com/ieshan/scheduler#Cron) — 5-field cron expression (minute hour dom month dow).
- [`Every(5 * time.Minute)`](https://pkg.go.dev/github.com/ieshan/scheduler#Every) — fixed interval.
- [`At(time.Now().Add(time.Hour))`](https://pkg.go.dev/github.com/ieshan/scheduler#At) — one-shot execution.

```go
// Daily at 9 AM
s, _ := scheduler.Cron("0 9 * * *")

// Every 5 minutes
s := scheduler.Every(5 * time.Minute)

// Once, 1 hour from now
s := scheduler.At(time.Now().Add(time.Hour))
```

### Executors

Implement [`JobExecutor`](https://pkg.go.dev/github.com/ieshan/scheduler#JobExecutor) to handle specific job types:

```go
type shellExecutor struct{}

func (e *shellExecutor) Execute(ctx context.Context, job *scheduler.Job) (*scheduler.JobResult, error) {
    // Respect ctx cancellation.
    cmd := exec.CommandContext(ctx, "sh", "-c", job.Script)
    out, err := cmd.CombinedOutput()
    if err != nil {
        return nil, fmt.Errorf("shell: %w", err)
    }
    return &scheduler.JobResult{
        Status: scheduler.StatusSuccess,
        Output: string(out),
    }, nil
}
```

Register executors in the scheduler config:

```go
scheduler.Config{
    Executors: map[string]scheduler.JobExecutor{
        "shell": &shellExecutor{},
        "agent": &agentExecutor{},
    },
}
```

### Store

The scheduler persists jobs via [`JobStore`](https://pkg.go.dev/github.com/ieshan/scheduler#JobStore). For production, implement the interface with your database. For testing, use the built-in [`InMemoryJobStore`](https://pkg.go.dev/github.com/ieshan/scheduler#InMemoryJobStore):

```go
store := scheduler.NewInMemoryJobStore()
```

The store interface has five methods:

- `List(ctx) ([]Job, error)` — return all jobs.
- `Get(ctx, id) (*Job, error)` — return a single job.
- `Save(ctx, *Job) error` — create or overwrite a job.
- `Delete(ctx, id) error` — remove a job.
- `UpdateState(ctx, id, JobState) error` — update runtime state after execution.

### Delivery

After a job completes, the scheduler optionally routes the result via [`DeliveryService`](https://pkg.go.dev/github.com/ieshan/scheduler#DeliveryService). The built-in [`RouterDelivery`](https://pkg.go.dev/github.com/ieshan/scheduler#RouterDelivery) routes by channel-key prefix:

```go
tg := &telegramSender{} // implements MessageSender

delivery := scheduler.NewRouterDelivery(
    map[string]scheduler.MessageSender{"tg": tg},
    redactSecrets, // optional credential-redaction function
)

sched := scheduler.New(cfg, scheduler.WithDelivery(delivery))
```

If delivery is not configured, results are still written to the store but not sent anywhere.

### Retry Backoff

Executors can use [`RetryBackoff`](https://pkg.go.dev/github.com/ieshan/scheduler#RetryBackoff) for jittered exponential backoff:

```go
delay := scheduler.RetryBackoff(time.Second, attempt)
// attempt 0: 0.5s – 1.5s
// attempt 1: 1s – 3s
// attempt 2: 2s – 6s
```

## Graceful Shutdown

`Stop` cancels the context passed to `Start`, signals all workers to exit, and waits for in-flight jobs to finish. It is safe to call multiple times.

For production deployments, wire `Stop` to OS signals so the scheduler shuts down cleanly on `SIGINT`/`SIGTERM`:

```go
package main

import (
    "context"
    "os"
    "os/signal"
    "syscall"

    "github.com/ieshan/scheduler"
)

func main() {
    store := scheduler.NewInMemoryJobStore()
    sched := scheduler.New(scheduler.Config{
        Store:     store,
        Executors: map[string]scheduler.JobExecutor{"print": &printExecutor{}},
    })

    // Start the scheduler in a goroutine.
    go sched.Start(context.Background())

    // Block until a termination signal is received.
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    // Stop waits for all in-flight jobs to complete.
    sched.Stop()
}
```

Key behaviors:

- **In-flight jobs finish** — `Stop` blocks on a `sync.WaitGroup` until every worker goroutine exits.
- **Context is cancelled** — Executors receive `context.Canceled` and should abort long-running work promptly.
- **No new dispatches** — Once `Stop` is called, no further jobs are enqueued.
- **Idempotent** — Calling `Stop` more than once is safe; only the first call takes effect.

## Testing

The scheduler is designed for deterministic testing:

- **Fake clock** — inject `WithNowFunc` to control time.
- **In-memory store** — fast, isolated, no database setup.
- **Context cancellation** — verify graceful shutdown and timeout behavior.

```go
func TestMyScheduler(t *testing.T) {
    t.Parallel()
    ctx := t.Context()

    store := scheduler.NewInMemoryJobStore()
    exec := &myTestExecutor{}

    sched := scheduler.New(scheduler.Config{
        Store:         store,
        Executors:     map[string]scheduler.JobExecutor{"test": exec},
        PollInterval:  50 * time.Millisecond,
        MaxConcurrent: 2,
    })

    go sched.Start(ctx)
    defer sched.Stop()

    // Add a job and wake the scheduler.
    store.Save(ctx, &scheduler.Job{...})
    sched.Wake()

    // Assert executor was called...
}
```

Run the test suite:

```bash
go test ./...
```

## Design Decisions

- **Polling vs. push** — The scheduler polls the store rather than maintaining an in-memory heap. This makes it resilient to external job changes and simplifies distributed deployments (at the cost of ~30s latency by default).
- **Worker pool** — A fixed pool of worker goroutines processes jobs from a queue, bounded by `MaxConcurrent`. This prevents thundering-herd goroutine creation when many jobs are due simultaneously.
- **Context per Start, not per job** — `Start` takes a single context; stopping the scheduler cancels it, signalling all executors to abort.

## API Stability

This package follows semantic versioning. The public API is stable within major versions. Go 1.26+ is required for the `new(expr)` syntax used internally and for modern standard-library features (`slog`, `math/rand/v2`, `t.Context()`).

## License

MPL-2.0
