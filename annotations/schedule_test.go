package annotations_test

import (
	"bytes"
	"context"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/titpetric/phpscript/annotations"
)

func TestParseSchedules(t *testing.T) {
	src := []byte(`<?php
// @schedule daily -- prune
// @schedule every 5 minutes -- sync
// @schedule every weekday
// @schedule 2 times per hour -- tick
// @schedule every sunday
echo "// @schedule hourly";
`)
	got := annotations.ParseSchedules(src)
	if len(got) != 5 {
		t.Fatalf("schedules = %+v", got)
	}
	if got[0].Raw != "daily" || got[0].Args[0] != "prune" || got[0].Every != 0 {
		t.Fatalf("daily = %+v", got[0])
	}
	if got[1].Every != 5*time.Minute || got[1].Args[0] != "sync" {
		t.Fatalf("every 5 minutes = %+v", got[1])
	}
	if len(got[2].Weekdays) != 5 {
		t.Fatalf("weekday = %+v", got[2])
	}
	if got[3].Every != 30*time.Minute {
		t.Fatalf("2 times per hour = %v", got[3].Every)
	}
	if len(got[4].Weekdays) != 1 || got[4].Weekdays[0] != time.Sunday {
		t.Fatalf("sunday = %+v", got[4])
	}
}

func TestScheduleNext(t *testing.T) {
	now := time.Date(2026, 8, 18, 15, 4, 5, 0, time.UTC)
	interval := annotations.Schedule{Every: 5 * time.Minute}
	if got := interval.Next(now); !got.Equal(now.Add(5 * time.Minute)) {
		t.Fatalf("interval next = %v", got)
	}
	parsed := annotations.ParseSchedules([]byte("<?php\n// @schedule daily\n"))
	if len(parsed) != 1 {
		t.Fatal(parsed)
	}
	got := parsed[0].Next(now)
	want := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("daily next = %v, want %v", got, want)
	}
}

func TestSchedulerRunsWithArgv(t *testing.T) {
	root := fstest.MapFS{
		"job.php": {Data: []byte("<?php\n// @schedule every 1 seconds -- prune\necho $argv[1];\n")},
	}
	var buf bytes.Buffer
	s := annotations.NewScheduler(root, annotations.WithOutput(&buf))
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Drive one execution through the same path Start uses by calling after a tick.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if buf.String() == "prune" {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("output = %q, want prune", buf.String())
}

func TestSchedulerStartScansFS(t *testing.T) {
	root := fstest.MapFS{
		"plain.php": {Data: []byte("<?php echo 1;")},
		"cron.php":  {Data: []byte("<?php\n// @schedule daily\n")},
	}
	if err := annotations.NewScheduler(fs.FS(root)).Start(context.Background()); err != nil {
		t.Fatal(err)
	}
}
