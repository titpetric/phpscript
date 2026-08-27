// Package time exposes Go's time values to PHP without wrapping their method
// sets. Constructors and package functions are registered explicitly; methods
// such as Time.Format, Time.Add, Duration.Minutes and Location.String are
// provided by the runtime's normal Go-value method dispatch.
//
// Two consequences of dispatching to Go's own methods are worth stating, since
// no wrapper is there to soften them:
//
//   - A duration argument is nanoseconds when written as an integer, so
//     $t->add(86400) advances by 86.4 microseconds. The runtime coerces a
//     string to a time.Duration, so $t->add("24h") is the spelling that means
//     what a PHP author reading time()+86400 expects.
//   - A method Go declares with several results returns a PHP list, read with
//     list($year, $week) = $t->iso_week(). The short [$a, $b] = spelling does
//     not parse in this runtime.
package time

import (
	"fmt"
	"strings"
	stdtime "time"

	"github.com/titpetric/phpscript/runner"
)

func init() {
	runner.RegisterBinding(Register)
}

// clock holds the default location for one PHP runtime. It deliberately does
// not mutate time.Local or process environment, which would leak one request's
// choice into concurrent runtimes.
type clock struct {
	location *stdtime.Location
}

// Register installs DateTime package functions and the Time, Time\Duration and
// Time\Location value classes on rt.
func Register(rt *runner.Runtime) {
	c := &clock{location: stdtime.Local}

	// The layouts worth naming, under Go's own names for them. A layout is an
	// ordinary string and any of them can be written out, but the one a script
	// should reach for by default deserves to be spelled rather than
	// remembered: TIME_RFC3339 is the only layout here that carries the offset,
	// so it is the one that survives a round trip through a database column or
	// a JSON payload. A namespaced constant does not parse in this runtime, so
	// the Time\ prefix the classes use is spelled TIME_ here.
	rt.SetConst("TIME_RFC3339", stdtime.RFC3339)
	rt.SetConst("TIME_RFC3339_NANO", stdtime.RFC3339Nano)
	rt.SetConst("TIME_DATETIME", stdtime.DateTime)
	rt.SetConst("TIME_DATE_ONLY", stdtime.DateOnly)
	rt.SetConst("TIME_TIME_ONLY", stdtime.TimeOnly)

	// Time constructs the current instant in the runtime's default location.
	rt.RegisterConstructor("Time", func() stdtime.Time {
		return c.now()
	})

	// Time\Duration constructs a Go duration from a duration string such as
	// "30m", or from an integer nanosecond count.
	rt.RegisterConstructor("Time\\Duration", newDuration)

	// Time\Location loads an IANA location such as "Europe/Ljubljana".
	rt.RegisterConstructor("Time\\Location", stdtime.LoadLocation)

	// set_timezone selects the default location used by Time construction,
	// parsing and Unix conversion in this runtime. It accepts either an IANA
	// location name or a Time\Location value.
	rt.RegisterFunc("set_timezone", c.setTimezone)

	// DateTime::now returns the current instant in the default location.
	rt.RegisterFunc("DateTime::now", c.now)

	// DateTime::parse parses $value with the Go $layout in the default location.
	rt.RegisterFunc("DateTime::parse", c.parse)

	// DateTime::parse_in_location parses $value with an explicit Time\Location.
	rt.RegisterFunc("DateTime::parse_in_location", stdtime.ParseInLocation)

	// DateTime::date constructs an instant in the default location.
	rt.RegisterFunc("DateTime::date", c.date)

	// DateTime::unix constructs an instant from seconds and nanoseconds since the Unix epoch.
	rt.RegisterFunc("DateTime::unix", c.unix)

	// DateTime::unix_milli constructs an instant from milliseconds since the Unix epoch.
	rt.RegisterFunc("DateTime::unix_milli", c.unixMilli)

	// DateTime::unix_micro constructs an instant from microseconds since the Unix epoch.
	rt.RegisterFunc("DateTime::unix_micro", c.unixMicro)

	// DateTime::since returns the duration elapsed since $time.
	rt.RegisterFunc("DateTime::since", stdtime.Since)

	// DateTime::until returns the duration until $time.
	rt.RegisterFunc("DateTime::until", stdtime.Until)

	// Time\Duration::parse parses a Go duration string such as "1h30m".
	rt.RegisterFunc("Time\\Duration::parse", stdtime.ParseDuration)

	// Time\Location::load loads an IANA location by name.
	rt.RegisterFunc("Time\\Location::load", stdtime.LoadLocation)

	// Time\Location::fixed constructs a location with a fixed offset in seconds east of UTC.
	rt.RegisterFunc("Time\\Location::fixed", stdtime.FixedZone)

	// Time\Location::utc returns UTC.
	rt.RegisterFunc("Time\\Location::utc", func() *stdtime.Location { return stdtime.UTC })

	// Time\Location::local returns the process-local location.
	rt.RegisterFunc("Time\\Location::local", func() *stdtime.Location { return stdtime.Local })
}

func newDuration(value any) (stdtime.Duration, error) {
	switch v := value.(type) {
	case string:
		return stdtime.ParseDuration(strings.TrimSpace(v))
	case stdtime.Duration:
		return v, nil
	case int:
		return stdtime.Duration(v), nil
	case int64:
		return stdtime.Duration(v), nil
	case float64:
		return stdtime.Duration(v), nil
	case nil:
		return 0, nil
	default:
		return 0, fmt.Errorf("Time\\Duration: expected a duration string or nanosecond count, got %T", value)
	}
}

func (c *clock) now() stdtime.Time {
	return stdtime.Now().In(c.location)
}

func (c *clock) parse(layout, value string) (stdtime.Time, error) {
	return stdtime.ParseInLocation(layout, value, c.location)
}

func (c *clock) date(year int, month stdtime.Month, day, hour, min, sec, nsec int) stdtime.Time {
	return stdtime.Date(year, month, day, hour, min, sec, nsec, c.location)
}

func (c *clock) unix(sec, nsec int64) stdtime.Time {
	return stdtime.Unix(sec, nsec).In(c.location)
}

func (c *clock) unixMilli(msec int64) stdtime.Time {
	return stdtime.UnixMilli(msec).In(c.location)
}

func (c *clock) unixMicro(usec int64) stdtime.Time {
	return stdtime.UnixMicro(usec).In(c.location)
}

func (c *clock) setTimezone(value any) (bool, error) {
	location, err := location(value)
	if err != nil {
		return false, err
	}
	c.location = location
	return true, nil
}

func location(value any) (*stdtime.Location, error) {
	switch v := value.(type) {
	case *stdtime.Location:
		if v == nil {
			return nil, fmt.Errorf("set_timezone: location must not be nil")
		}
		return v, nil
	case string:
		return stdtime.LoadLocation(strings.TrimSpace(v))
	default:
		return nil, fmt.Errorf("set_timezone: expected a location name or Time\\Location, got %T", value)
	}
}
