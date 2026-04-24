package scheduler

import (
	"testing"
	"time"
)

func TestEverySchedule_NextTick(t *testing.T) {
	t.Parallel()
	s := Every(5 * time.Minute)
	now := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	next := s.NextTick(now)
	want := now.Add(5 * time.Minute)
	if !next.Equal(want) {
		t.Errorf("NextTick = %v, want %v", next, want)
	}
}

func TestAtSchedule_NextTick_Future(t *testing.T) {
	t.Parallel()
	target := time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC)
	s := At(target)
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	next := s.NextTick(now)
	if !next.Equal(target) {
		t.Errorf("NextTick = %v, want %v", next, target)
	}
}

func TestAtSchedule_NextTick_Past(t *testing.T) {
	t.Parallel()
	target := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	s := At(target)
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	next := s.NextTick(now)
	if !next.IsZero() {
		t.Errorf("past AtSchedule should return zero time, got %v", next)
	}
}

func TestEverySchedule_Type(t *testing.T) {
	t.Parallel()
	s := Every(time.Minute)
	if s.Type() != "every" {
		t.Errorf("Type() = %q, want %q", s.Type(), "every")
	}
}

func TestAtSchedule_Type(t *testing.T) {
	t.Parallel()
	s := At(time.Now().Add(time.Hour))
	if s.Type() != "at" {
		t.Errorf("Type() = %q, want %q", s.Type(), "at")
	}
}
