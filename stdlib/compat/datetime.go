package compat

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/titpetric/phpscript/runner"
)

// registerDatetime installs the clock functions scripts time themselves with,
// and the simplistic strtotime/date shims. The clock pair answers "how long
// did that take" in the epoch integers PHP spells them in; the shims cover the
// epoch-and-layout corner of PHP's date family, and no more of it: strtotime
// matches a fixed list of layouts rather than reading English, and date knows
// only the numeric format characters. Anything richer is stdlib/time, where
// the value is a Go time.Time rather than an int; the boundary is recorded in
// docs/design.md under "Dates and times".
func registerDatetime(rt *runner.Runtime) {
	// time returns the current Unix timestamp in seconds.
	rt.RegisterFunc("time", func() int64 {
		return time.Now().Unix()
	})

	// microtime(true) returns seconds as a float; the argumentless form
	// returns PHP's historical "msec sec" string.
	rt.RegisterFunc("microtime", func(asFloat ...bool) any {
		now := time.Now()
		if len(asFloat) > 0 && asFloat[0] {
			return float64(now.UnixNano()) / float64(time.Second)
		}
		sec := now.Unix()
		frac := float64(now.Nanosecond()) / float64(time.Second)
		return fmt.Sprintf("%.8f %d", frac, sec)
	})

	// strtotime parses a datetime string against a fixed list of layouts, most
	// specific first, and returns the Unix timestamp, or false when none match.
	rt.RegisterFunc("strtotime", strtotime)

	// date formats a Unix timestamp with PHP's numeric format characters.
	rt.RegisterFunc("date", date)
}

// strtotimeLayouts are the formats strtotime recognises, ordered most specific
// first so the winning layout is the one that read the most of the input: an
// offset and a fraction are claimed before the same instant written without
// them. A layout without an offset reads in the runtime's local timezone, as
// PHP reads it in date.timezone. English forms ("next thursday") stay out on
// purpose; see docs/design.md under "Dates and times".
var strtotimeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.999999999",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05.999999999",
	time.DateTime,
	"2006-01-02 15:04",
	time.DateOnly,
	time.RFC1123Z,
	time.RFC1123,
}

func strtotime(value string, base ...int64) any {
	value = strings.TrimSpace(value)
	switch {
	case value == "":
		return false
	case strings.EqualFold(value, "now"):
		if len(base) > 0 {
			return base[0]
		}
		return time.Now().Unix()
	case strings.HasPrefix(value, "@"):
		// PHP truncates a fractional epoch to whole seconds.
		if sec, err := strconv.ParseInt(value[1:], 10, 64); err == nil {
			return sec
		}
		if sec, err := strconv.ParseFloat(value[1:], 64); err == nil {
			return int64(sec)
		}
		return false
	}
	for _, layout := range strtotimeLayouts {
		if t, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return t.Unix()
		}
	}
	return false
}

func date(format string, timestamp ...int64) string {
	t := time.Now()
	if len(timestamp) > 0 {
		t = time.Unix(timestamp[0], 0)
	}
	t = t.In(time.Local)

	hour12 := t.Hour() % 12
	if hour12 == 0 {
		hour12 = 12
	}

	var b strings.Builder
	runes := []rune(format)
	for i := 0; i < len(runes); i++ {
		switch ch := runes[i]; ch {
		case '\\':
			if i+1 < len(runes) {
				i++
				b.WriteRune(runes[i])
			}
		case 'Y':
			b.WriteString(strconv.Itoa(t.Year()))
		case 'y':
			fmt.Fprintf(&b, "%02d", t.Year()%100)
		case 'm':
			fmt.Fprintf(&b, "%02d", int(t.Month()))
		case 'n':
			b.WriteString(strconv.Itoa(int(t.Month())))
		case 'd':
			fmt.Fprintf(&b, "%02d", t.Day())
		case 'j':
			b.WriteString(strconv.Itoa(t.Day()))
		case 'H':
			fmt.Fprintf(&b, "%02d", t.Hour())
		case 'G':
			b.WriteString(strconv.Itoa(t.Hour()))
		case 'h':
			fmt.Fprintf(&b, "%02d", hour12)
		case 'g':
			b.WriteString(strconv.Itoa(hour12))
		case 'i':
			fmt.Fprintf(&b, "%02d", t.Minute())
		case 's':
			fmt.Fprintf(&b, "%02d", t.Second())
		case 'U':
			b.WriteString(strconv.FormatInt(t.Unix(), 10))
		default:
			// PHP writes an unrecognised character through unchanged; the
			// word-and-zone characters (D, M, T, e) are stdlib/time's job.
			b.WriteRune(ch)
		}
	}
	return b.String()
}
