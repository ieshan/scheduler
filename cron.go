package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// cronSchedule fires based on a 5-field cron expression.
type cronSchedule struct {
	expr    string
	minutes []int // 0-59
	hours   []int // 0-23
	doms    []int // 1-31
	months  []int // 1-12
	dows    []int // 0-6 (0=Sunday)
}

// Cron creates a cron [Schedule] from a 5-field expression: minute hour dom month dow.
// Returns an error if the expression has the wrong number of fields or values are out of range.
//
// Examples:
//   - "0 9 * * *" — every day at 9:00 AM
//   - "0 9 * * 1-5" — weekdays at 9:00 AM
//   - "*/5 * * * *" — every 5 minutes
//   - "0 0 1 * *" — first day of each month at midnight
func Cron(expr string) (Schedule, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron: expected 5 fields, got %d", len(fields))
	}

	minutes, err := parseField(fields[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("cron: minute: %w", err)
	}
	hours, err := parseField(fields[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("cron: hour: %w", err)
	}
	doms, err := parseField(fields[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("cron: day-of-month: %w", err)
	}
	months, err := parseField(fields[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("cron: month: %w", err)
	}
	dows, err := parseField(fields[4], 0, 6)
	if err != nil {
		return nil, fmt.Errorf("cron: day-of-week: %w", err)
	}

	return &cronSchedule{
		expr: expr, minutes: minutes, hours: hours,
		doms: doms, months: months, dows: dows,
	}, nil
}

func (s *cronSchedule) Type() string { return "cron" }

func (s *cronSchedule) NextTick(now time.Time) time.Time {
	// Start from the next minute
	t := now.Truncate(time.Minute).Add(time.Minute)
	limit := now.Add(366 * 24 * time.Hour)

	for t.Before(limit) {
		// Try to find a match starting from t
		next := s.findNext(t)
		if !next.IsZero() {
			return next
		}
		// No match in current month, move to first day of next month
		t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
	}
	return time.Time{}
}

// findNext finds the next match starting from the given time, but only within the same month
func (s *cronSchedule) findNext(start time.Time) time.Time {
	// Get valid months
	for _, month := range s.months {
		if month < int(start.Month()) {
			continue
		}

		// For months after start month, begin at day 1
		dayStart := 1
		if month == int(start.Month()) {
			dayStart = start.Day()
		}

		// Get valid days for this month
		validDays := s.validDaysInMonth(month, start.Year())

		for _, day := range validDays {
			if day < dayStart {
				continue
			}

			// For days after start day, begin at hour 0
			hourStart := 0
			if day == dayStart && month == int(start.Month()) {
				hourStart = start.Hour()
			}

			for _, hour := range s.hours {
				if hour < hourStart {
					continue
				}

				// For same hour, begin at next minute
				minStart := 0
				if hour == hourStart && day == dayStart && month == int(start.Month()) {
					minStart = start.Minute()
				}

				for _, minute := range s.minutes {
					if minute < minStart {
						continue
					}

					// Found a match
					return time.Date(start.Year(), time.Month(month), day, hour, minute, 0, 0, start.Location())
				}
				minStart = 0 // Reset for subsequent hours
			}
			hourStart = 0 // Reset for subsequent days
		}
		dayStart = 1 // Reset for subsequent months
	}
	return time.Time{}
}

// validDaysInMonth returns valid days considering both day-of-month and day-of-week constraints
func (s *cronSchedule) validDaysInMonth(month, year int) []int {
	// Get days in month
	daysInMonth := 31
	switch time.Month(month) {
	case time.April, time.June, time.September, time.November:
		daysInMonth = 30
	case time.February:
		daysInMonth = 28
		if isLeapYear(year) {
			daysInMonth = 29
		}
	}

	// If both dom and dow are "*", all days are valid
	domAll := len(s.doms) == daysInMonth && s.doms[0] == 1 && s.doms[len(s.doms)-1] == daysInMonth
	dowAll := len(s.dows) == 7 && s.dows[0] == 0 && s.dows[len(s.dows)-1] == 6

	if domAll && dowAll {
		return s.doms
	}

	// Collect valid days
	var valid []int
	for day := 1; day <= daysInMonth; day++ {
		t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)

		domMatch := contains(s.doms, day)
		dowMatch := contains(s.dows, int(t.Weekday()))

		// Standard cron behavior: if both dom and dow are restricted (not *),
		// match if either matches. Otherwise match if both match.
		if !domAll && !dowAll {
			// Both restricted - OR logic
			if domMatch || dowMatch {
				valid = append(valid, day)
			}
		} else {
			// At least one is * - AND logic (both must match, but one is always true)
			if domMatch && dowMatch {
				valid = append(valid, day)
			}
		}
	}
	return valid
}

func isLeapYear(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

func (s *cronSchedule) matches(t time.Time) bool {
	return contains(s.minutes, t.Minute()) &&
		contains(s.hours, t.Hour()) &&
		contains(s.doms, t.Day()) &&
		contains(s.months, int(t.Month())) &&
		contains(s.dows, int(t.Weekday()))
}

func contains(vals []int, v int) bool {
	for _, x := range vals {
		if x == v {
			return true
		}
	}
	return false
}

func parseField(field string, min, max int) ([]int, error) {
	if field == "*" {
		vals := make([]int, max-min+1)
		for i := range vals {
			vals[i] = min + i
		}
		return vals, nil
	}

	// Handle */N step
	if strings.HasPrefix(field, "*/") {
		step, err := strconv.Atoi(field[2:])
		if err != nil || step <= 0 {
			return nil, fmt.Errorf("invalid step: %q", field)
		}
		var vals []int
		for i := min; i <= max; i += step {
			vals = append(vals, i)
		}
		return vals, nil
	}

	// Single value
	v, err := strconv.Atoi(field)
	if err != nil {
		return nil, fmt.Errorf("invalid value: %q", field)
	}
	if v < min || v > max {
		return nil, fmt.Errorf("value %d out of range [%d, %d]", v, min, max)
	}
	return []int{v}, nil
}
