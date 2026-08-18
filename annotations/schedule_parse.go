package annotations

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule is one `@schedule` annotation: when to run, and argv after `--`.
type Schedule struct {
	Raw      string
	Args     []string
	Every    time.Duration
	Weekdays []time.Weekday
	MonthDay int
	Align    align
}

type align int

const (
	alignInterval align = iota
	alignDay
)

// ParseSchedules returns the @schedule declarations found in src. A line may
// carry `-- args` after the interval spec; those become $argv[1:].
func ParseSchedules(src []byte) []Schedule {
	var out []Schedule
	for line := range strings.SplitSeq(string(src), "\n") {
		text, ok := comment(line)
		if !ok {
			continue
		}
		name, fields := tag(text)
		if name != "@schedule" || len(fields) == 0 {
			continue
		}
		spec, args := splitScheduleArgs(fields)
		job, err := parseScheduleSpec(spec)
		if err != nil {
			continue
		}
		job.Raw = spec
		job.Args = args
		out = append(out, job)
	}
	return out
}

func splitScheduleArgs(fields []string) (string, []string) {
	for i, field := range fields {
		if field == "--" {
			return strings.Join(fields[:i], " "), append([]string(nil), fields[i+1:]...)
		}
	}
	return strings.Join(fields, " "), nil
}

func parseScheduleSpec(spec string) (Schedule, error) {
	fields := strings.Fields(strings.ToLower(spec))
	if len(fields) == 0 {
		return Schedule{}, fmt.Errorf("empty schedule")
	}
	switch {
	case eq(fields, "hourly"):
		return Schedule{Every: time.Hour}, nil
	case eq(fields, "daily"):
		return Schedule{Align: alignDay}, nil
	case eq(fields, "weekly"):
		return Schedule{Align: alignDay, Weekdays: []time.Weekday{time.Sunday}}, nil
	case eq(fields, "monthly"):
		return Schedule{Align: alignDay, MonthDay: 1}, nil
	case eq(fields, "every", "weekday"):
		return Schedule{Align: alignDay, Weekdays: weekdays}, nil
	case len(fields) == 2 && fields[0] == "every":
		if day, ok := weekdayName(fields[1]); ok {
			return Schedule{Align: alignDay, Weekdays: []time.Weekday{day}}, nil
		}
	case len(fields) == 3 && fields[0] == "every":
		n, err := strconv.Atoi(fields[1])
		if err != nil || n <= 0 {
			return Schedule{}, fmt.Errorf("invalid interval count %q", fields[1])
		}
		unit, err := durationUnit(fields[2])
		if err != nil {
			return Schedule{}, err
		}
		return Schedule{Every: time.Duration(n) * unit}, nil
	case len(fields) == 4 && fields[1] == "times" && fields[2] == "per":
		n, err := strconv.Atoi(fields[0])
		if err != nil || n <= 0 {
			return Schedule{}, fmt.Errorf("invalid times count %q", fields[0])
		}
		switch fields[3] {
		case "hour", "hours":
			return Schedule{Every: time.Hour / time.Duration(n)}, nil
		case "day", "days":
			return Schedule{Every: 24 * time.Hour / time.Duration(n)}, nil
		}
	}
	return Schedule{}, fmt.Errorf("unknown schedule %q", spec)
}

func eq(fields []string, want ...string) bool {
	if len(fields) != len(want) {
		return false
	}
	for i := range fields {
		if fields[i] != want[i] {
			return false
		}
	}
	return true
}

func durationUnit(name string) (time.Duration, error) {
	switch name {
	case "second", "seconds":
		return time.Second, nil
	case "minute", "minutes":
		return time.Minute, nil
	case "hour", "hours":
		return time.Hour, nil
	default:
		return 0, fmt.Errorf("unknown unit %q", name)
	}
}

var weekdays = []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday}

func weekdayName(name string) (time.Weekday, bool) {
	switch name {
	case "sunday", "sun":
		return time.Sunday, true
	case "monday", "mon":
		return time.Monday, true
	case "tuesday", "tue":
		return time.Tuesday, true
	case "wednesday", "wed":
		return time.Wednesday, true
	case "thursday", "thu":
		return time.Thursday, true
	case "friday", "fri":
		return time.Friday, true
	case "saturday", "sat":
		return time.Saturday, true
	}
	return 0, false
}

// Next returns the first instant strictly after t at which the job should run.
func (s Schedule) Next(t time.Time) time.Time {
	if s.Every > 0 {
		return t.Add(s.Every)
	}
	candidate := nextMidnight(t)
	for i := 0; i < 62; i++ {
		if s.matchDay(candidate) {
			return candidate
		}
		candidate = candidate.Add(24 * time.Hour)
	}
	return t.Add(24 * time.Hour)
}

func nextMidnight(t time.Time) time.Time {
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	if !day.After(t) {
		day = day.Add(24 * time.Hour)
	}
	return day
}

func (s Schedule) matchDay(t time.Time) bool {
	if s.MonthDay > 0 && t.Day() != s.MonthDay {
		return false
	}
	if len(s.Weekdays) == 0 {
		return true
	}
	for _, day := range s.Weekdays {
		if t.Weekday() == day {
			return true
		}
	}
	return false
}
