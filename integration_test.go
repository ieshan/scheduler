package scheduler

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// mockSender records the most recent Send call.
type mockSender struct {
	mu     sync.Mutex
	target string
	text   string
	called chan struct{}
	once   sync.Once
}

func newMockSender() *mockSender {
	return &mockSender{called: make(chan struct{})}
}

func (m *mockSender) Send(_ context.Context, target, text string) error {
	m.mu.Lock()
	m.target = target
	m.text = text
	m.mu.Unlock()
	m.once.Do(func() { close(m.called) })
	return nil
}

func (m *mockSender) Name() string { return "tg" }

// helloExecutor always returns "hello" as output.
type helloExecutor struct{}

func (helloExecutor) Execute(_ context.Context, job *Job) (*JobResult, error) {
	return &JobResult{
		Status:     StatusSuccess,
		Output:     "hello from job " + job.Name,
		ChannelKey: job.ChannelKey,
	}, nil
}

// Scenario X: TestRuntime_ScheduledJob_DeliversToTelegram — a scheduled job
// fires, its output is routed through RouterDelivery to a mock Telegram
// sender, and the sender receives the expected target and text.
func TestRuntime_ScheduledJob_DeliversToTelegram(t *testing.T) {
	t.Parallel()

	tg := newMockSender()
	delivery := NewRouterDelivery(
		map[string]MessageSender{"tg": tg},
		nil,
	)

	store := NewInMemoryJobStore()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	_ = store.Save(ctx, &Job{
		ID:           "x1",
		Name:         "greet",
		Enabled:      true,
		ExecutorType: "hello",
		ChannelKey:   "tg:42",
		Prompt:       "say hello",
		Schedule:     Every(50 * time.Millisecond),
		State:        JobState{NextRun: time.Now()},
	})

	s := New(Config{
		Store:         store,
		MaxConcurrent: 2,
		PollInterval:  30 * time.Millisecond,
		Executors:     map[string]JobExecutor{"hello": helloExecutor{}},
	}, WithDelivery(delivery))

	go s.Start(ctx)

	select {
	case <-tg.called:
	case <-ctx.Done():
		t.Fatal("timed out waiting for Telegram delivery")
	}
	s.Stop()

	tg.mu.Lock()
	gotTarget := tg.target
	gotText := tg.text
	tg.mu.Unlock()

	if gotTarget != "42" {
		t.Fatalf("target = %q; want 42", gotTarget)
	}
	if !strings.Contains(gotText, "hello") {
		t.Fatalf("text = %q; want to contain 'hello'", gotText)
	}
}
