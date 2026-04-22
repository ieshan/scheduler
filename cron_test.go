package scheduler

import (
	"testing"
	"time"
)

func TestCronSchedule_EveryMinute(t *testing.T) {
	t.Parallel()
	s, err := Cron("* * * * *")
	if err != nil {
		t.Fatalf("Cron: %v", err)
	}
	now := time.Date(2026, 4, 15, 10, 30, 0, 0, time.UTC)
	next := s.NextTick(now)
	want := time.Date(2026, 4, 15, 10, 31, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("NextTick = %v, want %v", next, want)
	}
}

func TestCronSchedule_EveryFiveMinutes(t *testing.T) {
	t.Parallel()
	s, err := Cron("*/5 * * * *")
	if err != nil {
		t.Fatalf("Cron: %v", err)
	}
	now := time.Date(2026, 4, 15, 10, 32, 0, 0, time.UTC)
	next := s.NextTick(now)
	want := time.Date(2026, 4, 15, 10, 35, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("NextTick = %v, want %v", next, want)
	}
}

func TestCronSchedule_DailyAt9AM(t *testing.T) {
	t.Parallel()
	s, err := Cron("0 9 * * *")
	if err != nil {
		t.Fatalf("Cron: %v", err)
	}
	// Before 9am — should be today at 9
	now := time.Date(2026, 4, 15, 8, 0, 0, 0, time.UTC)
	next := s.NextTick(now)
	want := time.Date(2026, 4, 15, 9, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("NextTick = %v, want %v", next, want)
	}
	// After 9am — should be tomorrow at 9
	now2 := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	next2 := s.NextTick(now2)
	want2 := time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC)
	if !next2.Equal(want2) {
		t.Errorf("NextTick after = %v, want %v", next2, want2)
	}
}

func TestCronSchedule_Invalid(t *testing.T) {
	t.Parallel()
	_, err := Cron("not a cron")
	if err == nil {
		t.Fatal("expected error for invalid cron expression")
	}
}

func TestCronSchedule_TooFewFields(t *testing.T) {
	t.Parallel()
	_, err := Cron("* * *")
	if err == nil {
		t.Fatal("expected error for 3-field expression")
	}
}

func BenchmarkCronSchedule_NextTick_Yearly(b *testing.B) {
	s, err := Cron("0 0 1 1 *") // Once per year
	if err != nil {
		b.Fatal(err)
	}
	now := time.Date(2026, 4, 15, 10, 30, 0, 0, time.UTC)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.NextTick(now)
	}
}

func BenchmarkCronSchedule_NextTick_Hourly(b *testing.B) {
	s, err := Cron("0 * * * *") // Every hour
	if err != nil {
		b.Fatal(err)
	}
	now := time.Date(2026, 4, 15, 10, 30, 0, 0, time.UTC)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.NextTick(now)
	}
}
