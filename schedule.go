package scheduler

import "time"

// Schedule determines when a job should run.
// Implementations include [Cron] (5-field expression), [Every] (fixed interval), and [At] (one-shot).
type Schedule interface {
	// NextTick returns the next time this schedule fires after now.
	// Returns zero time if the schedule will never fire again (e.g., a past [At] schedule).
	NextTick(now time.Time) time.Time

	// Type returns the schedule type name ("cron", "every", "at").
	// Used during persistence and reconstruction.
	Type() string
}

// everySchedule fires at fixed intervals.
type everySchedule struct {
	interval time.Duration
}

// Every creates a fixed-interval [Schedule] that fires every duration.
// For example, Every(5*time.Minute) fires every 5 minutes.
func Every(interval time.Duration) Schedule {
	return &everySchedule{interval: interval}
}

// NextTick implements [Schedule] by returning now + interval.
func (s *everySchedule) NextTick(now time.Time) time.Time {
	return now.Add(s.interval)
}

// Type returns "every" for fixed-interval schedules.
func (s *everySchedule) Type() string { return "every" }

// atSchedule fires once at a specific time.
type atSchedule struct {
	at time.Time
}

// At creates a one-shot [Schedule] that fires exactly once at time t.
// If t is in the past, [Schedule.NextTick] returns zero time and the job never fires.
// Typically used with [Job.DeleteAfterRun] to auto-disable after execution.
func At(t time.Time) Schedule {
	return &atSchedule{at: t}
}

// NextTick implements [Schedule] by returning the scheduled time if it is in the future.
// Returns zero time if the scheduled time has passed.
func (s *atSchedule) NextTick(now time.Time) time.Time {
	if s.at.After(now) {
		return s.at
	}
	return time.Time{} // past — never fires again
}

// Type returns "at" for one-shot schedules.
func (s *atSchedule) Type() string { return "at" }
