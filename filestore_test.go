package scheduler

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

var _ JobStore = (*FileJobStore)(nil)

func TestFileJobStore_SaveAndGet(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	fs := newMemFileSystem()
	fs.MkdirAll("/test", 0750)
	store := newFileJobStoreForTest(fs)

	// Create a job with "every" schedule
	job := &Job{
		ID:                 "j1",
		Name:               "test-job",
		ScheduleType:       "every",
		ScheduleExpression: "5m",
		Schedule:           Every(5 * time.Minute),
		Enabled:            true,
		ExecutorType:       "test",
	}

	if err := store.Save(ctx, job); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Get(ctx, "j1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if got.Name != "test-job" {
		t.Errorf("Name = %q, want test-job", got.Name)
	}
	if got.ScheduleType != "every" {
		t.Errorf("ScheduleType = %q, want every", got.ScheduleType)
	}

	// Verify ErrJobNotFound for missing job
	_, err = store.Get(ctx, "missing")
	if !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("expected ErrJobNotFound, got %v", err)
	}
}

func TestFileJobStore_List(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	fs := newMemFileSystem()
	fs.MkdirAll("/test", 0750)
	store := newFileJobStoreForTest(fs)

	store.Save(ctx, &Job{ID: "a", Enabled: true, Name: "job-a"})
	store.Save(ctx, &Job{ID: "b", Enabled: false, Name: "job-b"})
	store.Save(ctx, &Job{ID: "c", Enabled: true, Name: "job-c"})

	jobs, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(jobs) != 3 {
		t.Errorf("List len = %d, want 3", len(jobs))
	}

	// Verify all IDs are present
	ids := make(map[string]bool)
	for _, j := range jobs {
		ids[j.ID] = true
	}
	if !ids["a"] || !ids["b"] || !ids["c"] {
		t.Errorf("Not all job IDs present in list")
	}
}

func TestFileJobStore_Delete(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	fs := newMemFileSystem()
	fs.MkdirAll("/test", 0750)
	store := newFileJobStoreForTest(fs)

	store.Save(ctx, &Job{ID: "del", Name: "to-delete"})

	if err := store.Delete(ctx, "del"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := store.Get(ctx, "del")
	if err == nil {
		t.Fatal("expected error after delete")
	}
	if !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("expected ErrJobNotFound after delete, got %v", err)
	}

	// Verify ErrJobNotFound when deleting non-existent job
	err = store.Delete(ctx, "nonexistent")
	if !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("expected ErrJobNotFound for non-existent job, got %v", err)
	}
}

func TestFileJobStore_UpdateState(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	fs := newMemFileSystem()
	fs.MkdirAll("/test", 0750)
	store := newFileJobStoreForTest(fs)

	store.Save(ctx, &Job{ID: "s1", Name: "state-test"})

	err := store.UpdateState(ctx, "s1", JobState{
		LastStatus: StatusSuccess,
		RunCount:   1,
		LastOutput: "completed",
	})
	if err != nil {
		t.Fatalf("UpdateState: %v", err)
	}

	got, _ := store.Get(ctx, "s1")
	if got.State.LastStatus != StatusSuccess {
		t.Errorf("LastStatus = %q, want success", got.State.LastStatus)
	}
	if got.State.RunCount != 1 {
		t.Errorf("RunCount = %d, want 1", got.State.RunCount)
	}
	if got.State.LastOutput != "completed" {
		t.Errorf("LastOutput = %q, want completed", got.State.LastOutput)
	}

	// Verify ErrJobNotFound for non-existent job
	err = store.UpdateState(ctx, "nonexistent", JobState{})
	if !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("expected ErrJobNotFound for non-existent job, got %v", err)
	}
}

func TestFileJobStore_Load(t *testing.T) {
	t.Parallel()
	fs := newMemFileSystem()
	fs.MkdirAll("/test", 0750)

	// Pre-populate the filesystem with JSON data
	jsonData := `[
  {
    "id": "loaded1",
    "name": "loaded-job",
    "schedule_type": "cron",
    "schedule_expression": "0 9 * * *",
    "enabled": true,
    "executor_type": "test",
    "state": {
      "next_run": "2026-01-15T09:00:00Z",
      "last_status": "success",
      "run_count": 5
    }
  }
]`
	fs.WriteFile("/test/jobs.json", []byte(jsonData), 0640)

	store := newFileJobStoreForTest(fs)
	if err := store.load(); err != nil {
		t.Fatalf("load: %v", err)
	}

	// Verify job was loaded
	ctx := t.Context()
	job, err := store.Get(ctx, "loaded1")
	if err != nil {
		t.Fatalf("Get loaded job: %v", err)
	}
	if job.Name != "loaded-job" {
		t.Errorf("Name = %q, want loaded-job", job.Name)
	}
	if job.ScheduleType != "cron" {
		t.Errorf("ScheduleType = %q, want cron", job.ScheduleType)
	}
	if job.ScheduleExpression != "0 9 * * *" {
		t.Errorf("ScheduleExpression = %q, want 0 9 * * *", job.ScheduleExpression)
	}
	if job.State.RunCount != 5 {
		t.Errorf("RunCount = %d, want 5", job.State.RunCount)
	}

	// Verify Schedule was reconstructed
	if job.Schedule == nil {
		t.Error("Schedule should be reconstructed but is nil")
	} else if job.Schedule.Type() != "cron" {
		t.Errorf("Schedule.Type() = %q, want cron", job.Schedule.Type())
	}
}

func TestFileJobStore_LoadEmptyFile(t *testing.T) {
	t.Parallel()
	fs := newMemFileSystem()
	fs.MkdirAll("/test", 0750)

	// File doesn't exist yet - should start empty
	store := newFileJobStoreForTest(fs)
	if err := store.load(); err != nil {
		t.Fatalf("load with no file: %v", err)
	}

	ctx := t.Context()
	jobs, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("expected empty store, got %d jobs", len(jobs))
	}
}

func TestFileJobStore_LoadCorruptedFile(t *testing.T) {
	t.Parallel()
	fs := newMemFileSystem()
	fs.MkdirAll("/test", 0750)

	// Write corrupted JSON
	fs.WriteFile("/test/jobs.json", []byte("not valid json"), 0640)

	store := newFileJobStoreForTest(fs)
	err := store.load()
	if err == nil {
		t.Fatal("expected error for corrupted file")
	}
}

func TestFileJobStore_ScheduleRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	fs := newMemFileSystem()
	fs.MkdirAll("/test", 0750)
	store := newFileJobStoreForTest(fs)

	tests := []struct {
		name       string
		schedule   Schedule
		schedType  string
		expression string
	}{
		{
			name:       "cron",
			schedule:   mustCron("0 9 * * *"),
			schedType:  "cron",
			expression: "0 9 * * *",
		},
		{
			name:       "every",
			schedule:   Every(5 * time.Minute),
			schedType:  "every",
			expression: "5m",
		},
		{
			name:       "at",
			schedule:   At(time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC)),
			schedType:  "at",
			expression: "2026-01-15T09:00:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &Job{
				ID:                 "rt-" + tt.name,
				Name:               "roundtrip-" + tt.name,
				Schedule:           tt.schedule,
				ScheduleType:       tt.schedType,
				ScheduleExpression: tt.expression,
				Enabled:            true,
			}

			if err := store.Save(ctx, job); err != nil {
				t.Fatalf("Save: %v", err)
			}

			// Verify Schedule was persisted
			got, err := store.Get(ctx, job.ID)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if got.ScheduleExpression != tt.expression {
				t.Errorf("ScheduleExpression = %q, want %q", got.ScheduleExpression, tt.expression)
			}
		})
	}
}

func TestFileJobStore_LoadReconstructsSchedules(t *testing.T) {
	t.Parallel()
	fs := newMemFileSystem()
	fs.MkdirAll("/test", 0750)

	// Pre-populate with all schedule types
	jsonData := `[
  {
    "id": "cron-job",
    "name": "cron",
    "schedule_type": "cron",
    "schedule_expression": "0 */6 * * *",
    "enabled": true,
    "executor_type": "test",
    "state": {}
  },
  {
    "id": "every-job",
    "name": "every",
    "schedule_type": "every",
    "schedule_expression": "30m",
    "enabled": true,
    "executor_type": "test",
    "state": {}
  },
  {
    "id": "at-job",
    "name": "at",
    "schedule_type": "at",
    "schedule_expression": "2026-12-25T00:00:00Z",
    "enabled": true,
    "executor_type": "test",
    "state": {}
  }
]`
	fs.WriteFile("/test/jobs.json", []byte(jsonData), 0640)

	store := newFileJobStoreForTest(fs)
	if err := store.load(); err != nil {
		t.Fatalf("load: %v", err)
	}

	ctx := t.Context()

	// Verify cron schedule reconstruction
	cronJob, err := store.Get(ctx, "cron-job")
	if err != nil {
		t.Fatalf("Get cron-job: %v", err)
	}
	if cronJob.Schedule == nil {
		t.Error("cron schedule should be reconstructed")
	} else if cronJob.Schedule.Type() != "cron" {
		t.Errorf("cron Schedule.Type() = %q, want cron", cronJob.Schedule.Type())
	}

	// Verify every schedule reconstruction
	everyJob, err := store.Get(ctx, "every-job")
	if err != nil {
		t.Fatalf("Get every-job: %v", err)
	}
	if everyJob.Schedule == nil {
		t.Error("every schedule should be reconstructed")
	} else if everyJob.Schedule.Type() != "every" {
		t.Errorf("every Schedule.Type() = %q, want every", everyJob.Schedule.Type())
	}

	// Verify at schedule reconstruction
	atJob, err := store.Get(ctx, "at-job")
	if err != nil {
		t.Fatalf("Get at-job: %v", err)
	}
	if atJob.Schedule == nil {
		t.Error("at schedule should be reconstructed")
	} else if atJob.Schedule.Type() != "at" {
		t.Errorf("at Schedule.Type() = %q, want at", atJob.Schedule.Type())
	}
}

func TestFileJobStore_Concurrency(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	fs := newMemFileSystem()
	fs.MkdirAll("/test", 0750)
	store := newFileJobStoreForTest(fs)

	// Start with some jobs
	for i := 0; i < 5; i++ {
		store.Save(ctx, &Job{ID: fmt.Sprintf("job%d", i), Name: fmt.Sprintf("Job %d", i)})
	}

	// Run concurrent operations
	done := make(chan bool, 3)

	// Goroutine 1: Save new jobs
	go func() {
		for i := 5; i < 10; i++ {
			store.Save(ctx, &Job{ID: fmt.Sprintf("job%d", i), Name: fmt.Sprintf("Job %d", i)})
		}
		done <- true
	}()

	// Goroutine 2: Update state
	go func() {
		for i := 0; i < 5; i++ {
			store.UpdateState(ctx, fmt.Sprintf("job%d", i), JobState{RunCount: i + 1})
		}
		done <- true
	}()

	// Goroutine 3: List jobs
	go func() {
		for i := 0; i < 10; i++ {
			store.List(ctx)
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 3; i++ {
		<-done
	}

	// Verify final state is consistent
	jobs, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(jobs) != 10 {
		t.Errorf("expected 10 jobs after concurrent operations, got %d", len(jobs))
	}

	// Verify state updates were persisted
	job0, err := store.Get(ctx, "job0")
	if err != nil {
		t.Fatalf("Get job0: %v", err)
	}
	if job0.State.RunCount == 0 {
		t.Error("state updates should have been persisted")
	}
}

func TestFileJobStore_Isolation(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	fs := newMemFileSystem()
	fs.MkdirAll("/test", 0750)
	store := newFileJobStoreForTest(fs)

	// Save a job
	original := &Job{ID: "iso", Name: "original", Enabled: true}
	store.Save(ctx, original)

	// Modify the original job after saving
	original.Name = "modified"
	original.Enabled = false

	// Get the job from store - should see original values
	got, err := store.Get(ctx, "iso")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "original" {
		t.Errorf("Name = %q, want original (store should isolate modifications)", got.Name)
	}
	if !got.Enabled {
		t.Error("Enabled = false, want true (store should isolate modifications)")
	}

	// Modify the returned job
	got.Name = "modified-after-get"

	// Get again - should still see original
	got2, _ := store.Get(ctx, "iso")
	if got2.Name != "original" {
		t.Errorf("Name = %q, want original (returned copy should be independent)", got2.Name)
	}
}

func TestFileJobStore_DeleteAfterRun(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	fs := newMemFileSystem()
	fs.MkdirAll("/test", 0750)
	store := newFileJobStoreForTest(fs)

	job := &Job{
		ID:             "one-shot",
		Name:           "One Shot Job",
		DeleteAfterRun: true,
		Enabled:        true,
	}

	if err := store.Save(ctx, job); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Get(ctx, "one-shot")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.DeleteAfterRun {
		t.Error("DeleteAfterRun should be true")
	}
}

func TestFileJobStore_PayloadAndConfig(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	fs := newMemFileSystem()
	fs.MkdirAll("/test", 0750)
	store := newFileJobStoreForTest(fs)

	job := &Job{
		ID:           "payload-test",
		Name:         "Payload Test",
		Payload:      map[string]any{"key": "value", "num": float64(42)},
		ExecutorType: "test",
		Config: JobConfig{
			Timeout:      30 * time.Second,
			MaxRetries:   3,
			RetryBackoff: 5 * time.Second,
		},
		ChannelKey: "tg:123456",
		Prompt:     "Execute this task",
		Script:     "echo hello",
	}

	if err := store.Save(ctx, job); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Get(ctx, "payload-test")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	// Verify config
	if got.Config.Timeout != 30*time.Second {
		t.Errorf("Config.Timeout = %v, want 30s", got.Config.Timeout)
	}
	if got.Config.MaxRetries != 3 {
		t.Errorf("Config.MaxRetries = %d, want 3", got.Config.MaxRetries)
	}
	if got.Config.RetryBackoff != 5*time.Second {
		t.Errorf("Config.RetryBackoff = %v, want 5s", got.Config.RetryBackoff)
	}

	// Verify payload roundtrip
	payload, ok := got.Payload.(map[string]any)
	if !ok {
		t.Fatalf("Payload should be map[string]any, got %T", got.Payload)
	}
	if payload["key"] != "value" {
		t.Errorf("Payload[key] = %v, want 'value'", payload["key"])
	}
	if payload["num"] != float64(42) {
		t.Errorf("Payload[num] = %v, want 42", payload["num"])
	}

	// Verify other fields
	if got.ChannelKey != "tg:123456" {
		t.Errorf("ChannelKey = %q, want tg:123456", got.ChannelKey)
	}
	if got.Prompt != "Execute this task" {
		t.Errorf("Prompt = %q, want 'Execute this task'", got.Prompt)
	}
	if got.Script != "echo hello" {
		t.Errorf("Script = %q, want 'echo hello'", got.Script)
	}
}

func TestFileJobStore_NewFileJobStore_LoadsExisting(t *testing.T) {
	t.Parallel()
	// This test uses a real temp directory
	tempDir := t.TempDir()
	filepath := tempDir + "/jobs.json"

	// Pre-create a file
	data := []byte(`[{"id": "existing", "name": "Existing Job", "enabled": true, "executor_type": "test", "state": {}}]`)
	if err := os.WriteFile(filepath, data, 0640); err != nil {
		t.Fatalf("setup: %v", err)
	}

	store, err := NewFileJobStore(filepath)
	if err != nil {
		t.Fatalf("NewFileJobStore: %v", err)
	}

	ctx := t.Context()
	job, err := store.Get(ctx, "existing")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if job.Name != "Existing Job" {
		t.Errorf("Name = %q, want 'Existing Job'", job.Name)
	}
}

func TestFileJobStore_NewFileJobStore_CreatesNew(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	filepath := tempDir + "/newjobs.json"

	// File doesn't exist yet
	store, err := NewFileJobStore(filepath)
	if err != nil {
		t.Fatalf("NewFileJobStore: %v", err)
	}

	ctx := t.Context()
	jobs, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("expected empty store, got %d jobs", len(jobs))
	}
}

func mustCron(expr string) Schedule {
	s, err := Cron(expr)
	if err != nil {
		panic(err)
	}
	return s
}
