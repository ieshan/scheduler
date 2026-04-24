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
	tests := []struct {
		name string
		expr string
	}{
		{"wrong field count", "not a cron"},
		{"too few fields", "* * *"},
		{"too many fields", "* * * * * *"},
		{"non-numeric minute", "abc * * * *"},
		{"minute out of range", "60 * * * *"},
		{"hour out of range", "* 24 * * *"},
		{"dom out of range", "* * 32 * *"},
		{"month out of range", "* * * 13 *"},
		{"dow out of range", "* * * * 7"},
		{"zero step", "*/0 * * * *"},
		{"negative value", "-1 * * * *"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Cron(tt.expr)
			if err == nil {
				t.Fatalf("Cron(%q): expected error, got nil", tt.expr)
			}
		})
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

func TestCronSchedule_Matches(t *testing.T) {
	t.Parallel()
	s, err := Cron("0 9 * * 1") // Mondays at 9:00 AM
	if err != nil {
		t.Fatalf("Cron: %v", err)
	}

	cs := s.(*cronSchedule)

	// Should match Monday 9:00 AM
	monday9 := time.Date(2026, 4, 13, 9, 0, 0, 0, time.UTC) // Monday
	if !cs.matches(monday9) {
		t.Errorf("expected match for Monday 9:00 AM")
	}

	// Should NOT match Tuesday 9:00 AM
	tuesday9 := time.Date(2026, 4, 14, 9, 0, 0, 0, time.UTC) // Tuesday
	if cs.matches(tuesday9) {
		t.Errorf("should not match Tuesday 9:00 AM")
	}

	// Should NOT match Monday 10:00 AM
	monday10 := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC) // Monday
	if cs.matches(monday10) {
		t.Errorf("should not match Monday 10:00 AM")
	}
}

func TestCronSchedule_Type(t *testing.T) {
	t.Parallel()
	s, err := Cron("* * * * *")
	if err != nil {
		t.Fatalf("Cron: %v", err)
	}
	if s.Type() != "cron" {
		t.Errorf("Type() = %q, want %q", s.Type(), "cron")
	}
}

func TestCronSchedule_BothDomAndDowRestricted(t *testing.T) {
	t.Parallel()
	// "0 0 15 * 1" = midnight on the 15th OR on Mondays (OR logic when both restricted)
	s, err := Cron("0 0 15 * 1")
	if err != nil {
		t.Fatalf("Cron: %v", err)
	}
	// April 2026: 15th is a Wednesday, Mondays are 6, 13, 20, 27
	// First matching day from April 1 should be April 6 (Monday)
	now := time.Date(2026, 4, 1, 1, 0, 0, 0, time.UTC)
	next := s.NextTick(now)
	want := time.Date(2026, 4, 6, 0, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("NextTick = %v, want %v", next, want)
	}
}

func TestCronSchedule_LeapYear(t *testing.T) {
	t.Parallel()

	// Test Feb 29 on leap year
	leapFeb29 := time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC)
	s, err := Cron("0 12 29 2 *") // Feb 29 at 12:00
	if err != nil {
		t.Fatalf("Cron: %v", err)
	}

	cs := s.(*cronSchedule)
	if !cs.matches(leapFeb29) {
		t.Errorf("expected match for Feb 29, 2024 (leap year)")
	}

	// Verify NextTick works correctly across leap year boundary
	beforeLeap := time.Date(2024, 2, 28, 23, 0, 0, 0, time.UTC)
	next := s.NextTick(beforeLeap)
	want := time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("NextTick across leap day = %v, want %v", next, want)
	}
}
