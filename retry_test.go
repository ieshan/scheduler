package scheduler

import (
	"testing"
	"time"
)

func TestRetryBackoff_Exponential(t *testing.T) {
	t.Parallel()
	base := 100 * time.Millisecond
	delays := make([]time.Duration, 5)
	for i := range delays {
		delays[i] = RetryBackoff(base, i)
	}
	// Each should be >= previous (with jitter, at least base * 2^attempt * 0.5)
	for i := 1; i < len(delays); i++ {
		minExpected := base * time.Duration(1<<uint(i)) / 2
		if delays[i] < minExpected {
			t.Errorf("attempt %d: delay %v < min expected %v", i, delays[i], minExpected)
		}
	}
}

func TestRetryBackoff_ZeroAttempt(t *testing.T) {
	t.Parallel()
	d := RetryBackoff(time.Second, 0)
	// Should be between 0.5s and 1.5s (1s * 1 with jitter)
	if d < 500*time.Millisecond || d > 1500*time.Millisecond {
		t.Errorf("attempt 0 delay = %v, want ~1s", d)
	}
}

func TestRetryBackoff_ZeroBase(t *testing.T) {
	t.Parallel()
	d := RetryBackoff(0, 3)
	if d != 0 {
		t.Errorf("zero base should return zero, got %v", d)
	}
}

func TestRetryBackoff_JitterBounds(t *testing.T) {
	t.Parallel()
	base := time.Second
	for attempt := range 5 {
		minDur := base * time.Duration(1<<uint(attempt)) / 2
		maxDur := base * time.Duration(1<<uint(attempt)) * 3 / 2
		for range 50 {
			d := RetryBackoff(base, attempt)
			if d < minDur || d > maxDur {
				t.Errorf("attempt %d: delay %v outside [%v, %v]", attempt, d, minDur, maxDur)
			}
		}
	}
}
