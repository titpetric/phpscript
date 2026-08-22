package http

import (
	"fmt"
	"strings"
	"time"

	"github.com/titpetric/phpscript/model"
)

// The conversions a script's option array needs. PHP is dynamically typed and
// an option arrives as whatever the script wrote, so each one is read the way
// PHP would read it in that position rather than type-asserted.

// toString renders a value the way PHP renders it in a string context.
func toString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}

// toBool follows PHP truthiness for the values a script can hand an option:
// "false", "off", "no", "0" and the empty string are false, as is a zero
// number; anything else set is true.
func toBool(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "", "0", "false", "off", "no":
			return false
		}
		return true
	default:
		return toInt(value) != 0
	}
}

// toInt reads an integer option.
func toInt(value any) int64 {
	switch v := value.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	case bool:
		if v {
			return 1
		}
		return 0
	case string:
		var n int64
		fmt.Sscanf(v, "%d", &n)
		return n
	default:
		return 0
	}
}

// toDuration reads a timeout. A bare number is seconds, which is how PHP's own
// timeouts are spelled; a string with a unit ("500ms", "1m30s") is parsed as
// Go spells one, so a sub-second timeout is expressible without a fraction.
func toDuration(value any) time.Duration {
	switch v := value.(type) {
	case string:
		if parsed, err := time.ParseDuration(strings.TrimSpace(v)); err == nil {
			return parsed
		}
		return time.Duration(toInt(v)) * time.Second
	case float64:
		return time.Duration(v * float64(time.Second))
	default:
		return time.Duration(toInt(value)) * time.Second
	}
}

// toHeaders reads a headers option, which a script writes as an associative
// array of name to value.
func toHeaders(value any) (map[string]string, error) {
	if value == nil {
		return nil, nil
	}
	if !model.IsCollection(value) {
		return nil, fmt.Errorf("HTTP\\Client: headers must be an array")
	}
	headers := map[string]string{}
	model.RangeValues(value, func(key, val any) bool {
		headers[toString(key)] = toString(val)
		return true
	})
	return headers, nil
}
