package compat

import (
	"strconv"
	"testing"
	"time"
)

// epoch is a fixed instant, 2023-11-14 22:13:20 UTC. A test reading a local
// rendering back computes what it wants from time.Local and the assertions
// hold in whatever timezone the host runs in.
const epoch = int64(1700000000)

// TestStrtotime covers the three spellings strtotime answers without reading a
// layout, each matching what php prints: an @-prefixed epoch truncated to
// whole seconds, "now" resolved against the base the caller passed, and false
// for the empty string. False and not zero, because zero is a valid timestamp.
func TestStrtotime(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		value string
		base  []int64
		want  any
	}{
		{value: "@1700000000", want: epoch},
		{value: "@1700000000.75", want: epoch},
		{value: "  @1700000000  ", want: epoch},
		{value: "now", base: []int64{epoch}, want: epoch},
		{value: "NOW", base: []int64{epoch}, want: epoch},
		{value: "", want: false},
		{value: "   ", want: false},
		{value: "next thursday", want: false},
	} {
		if got := strtotime(test.value, test.base...); got != test.want {
			t.Errorf("strtotime(%q) = %#v, want %#v", test.value, got, test.want)
		}
	}
}

// TestStrtotimeReadsWhatDateWrote covers the layout list against the formatter
// beside it: every layout strtotime tries reads in the local timezone, and the
// shapes date() writes are among them and a timestamp rendered and read back
// is the timestamp it started as. A layout list that drifted from the
// formatter would break that round trip silently.
func TestStrtotimeReadsWhatDateWrote(t *testing.T) {
	t.Parallel()
	for _, format := range []string{"Y-m-d H:i:s", "Y-m-d", "Y-m-d\\TH:i:s"} {
		rendered := date(format, epoch)
		got := strtotime(rendered)

		want := any(epoch)
		if format == "Y-m-d" {
			// A date without a time reads as midnight local and the round
			// trip lands on the start of that day and not on the instant.
			year, month, day := time.Unix(epoch, 0).In(time.Local).Date()
			want = time.Date(year, month, day, 0, 0, 0, 0, time.Local).Unix()
		}
		if got != want {
			t.Errorf("strtotime(date(%q)) = %#v for %q, want %#v", format, got, rendered, want)
		}
	}
}

// TestDate covers the format characters this shim reads and the one rule for
// everything else: a backslash escapes the next character, U is the epoch
// itself, the 12-hour characters report 12 at midnight and never 0, and the
// shim writes a character it does not know through unchanged. php renders
// the word-and-zone characters; stdlib/time is where a script reaches those,
// and the passthrough is the recorded boundary between the two.
func TestDate(t *testing.T) {
	t.Parallel()
	local := time.Unix(epoch, 0).In(time.Local)
	hour12 := local.Hour() % 12
	if hour12 == 0 {
		hour12 = 12
	}

	for _, test := range []struct {
		format string
		want   string
	}{
		{format: "U", want: strconv.FormatInt(epoch, 10)},
		{format: "Y", want: strconv.Itoa(local.Year())},
		{format: "y", want: pad(local.Year() % 100)},
		{format: "m", want: pad(int(local.Month()))},
		{format: "n", want: strconv.Itoa(int(local.Month()))},
		{format: "d", want: pad(local.Day())},
		{format: "j", want: strconv.Itoa(local.Day())},
		{format: "H", want: pad(local.Hour())},
		{format: "G", want: strconv.Itoa(local.Hour())},
		{format: "h", want: pad(hour12)},
		{format: "g", want: strconv.Itoa(hour12)},
		{format: "i", want: pad(local.Minute())},
		{format: "s", want: pad(local.Second())},
		{format: `\Y`, want: "Y"},
		{format: `\\`, want: `\`},
		{format: "D", want: "D"},
		{format: "M", want: "M"},
		{format: "-/: ", want: "-/: "},
	} {
		if got := date(test.format, epoch); got != test.want {
			t.Errorf("date(%q) = %q, want %q", test.format, got, test.want)
		}
	}
}

// TestDateMidnightIsTwelve covers the one arithmetic in the formatter: PHP's
// g and h count 1 to 12 and the hour 0 renders as 12 and never as 0.
func TestDateMidnightIsTwelve(t *testing.T) {
	t.Parallel()
	midnight := time.Date(2023, time.November, 14, 0, 0, 0, 0, time.Local).Unix()
	if got := date("g", midnight); got != "12" {
		t.Errorf(`date("g") at midnight = %q, want "12"`, got)
	}
	if got := date("h", midnight); got != "12" {
		t.Errorf(`date("h") at midnight = %q, want "12"`, got)
	}
	if got := date("H", midnight); got != "00" {
		t.Errorf(`date("H") at midnight = %q, want "00"`, got)
	}
}

// pad renders a two-digit field the way the formatter's %02d does.
func pad(value int) string {
	if value < 10 {
		return "0" + strconv.Itoa(value)
	}
	return strconv.Itoa(value)
}
