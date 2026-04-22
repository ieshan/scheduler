package scheduler

import (
	"math/rand/v2"
	"time"
)

// RetryBackoff calculates a jittered exponential backoff duration.
// This is commonly used by [JobExecutor] implementations to determine
// how long to wait before retrying a failed job.
//
// Formula: base * 2^attempt * (0.5 + rand(0, 1))
//
// For example, with base=1s:
//   - attempt 0: 0.5s – 1.5s
//   - attempt 1: 1s – 3s
//   - attempt 2: 2s – 6s
//
// The jitter prevents thundering herd when multiple jobs retry simultaneously.
func RetryBackoff(base time.Duration, attempt int) time.Duration {
	exp := time.Duration(1 << uint(attempt))
	backoff := base * exp
	jitter := 0.5 + rand.Float64()
	return time.Duration(float64(backoff) * jitter)
}
