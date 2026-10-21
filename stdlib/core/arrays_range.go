package core

import (
	"math"

	"github.com/titpetric/phpscript/internal/phpval"
)

// phpRange builds the inclusive sequence from start to end. It counts the
// elements up front and derives every one of them from the start, the way PHP
// does, so a float step never accumulates rounding error across the range.
//
// The result is a presized []any: a range is a dense list by construction, so
// there is no key information for an *model.Array to carry (rule 4).
func phpRange(start, end any, step ...any) []any {
	var by any = int64(1)
	if len(step) > 0 && step[0] != nil {
		by = step[0]
	}

	if s, e, ok := rangeChars(start, end); ok {
		return rangeCharSeq(s, e, by)
	}
	if rangeIsFloat(start) || rangeIsFloat(end) || rangeStepIsFloat(by) {
		return rangeFloats(phpval.Float(start), phpval.Float(end), math.Abs(phpval.Float(by)))
	}
	return rangeInts(phpval.Int(start), phpval.Int(end), phpval.Int(by))
}

// rangeChars reports the character-range case: both endpoints are one-character
// strings, the rule PHP 8.3 onwards narrowed to.
func rangeChars(start, end any) (byte, byte, bool) {
	s, ok := start.(string)
	if !ok || len(s) != 1 {
		return 0, 0, false
	}
	e, ok := end.(string)
	if !ok || len(e) != 1 {
		return 0, 0, false
	}
	return s[0], e[0], true
}

func rangeCharSeq(s, e byte, step any) []any {
	p := int(phpval.Int(step))
	if p < 0 {
		p = -p
	}
	if p == 0 {
		p = 1
	}
	from, to := int(s), int(e)
	span := to - from
	if span < 0 {
		span = -span
	}
	n := span/p + 1
	out := make([]any, 0, n)
	for i := 0; i < n; i++ {
		if from <= to {
			out = append(out, string([]byte{byte(from + i*p)}))
			continue
		}
		out = append(out, string([]byte{byte(from - i*p)}))
	}
	return out
}

// rangeIsFloat reports whether an endpoint forces a float range. A float
// endpoint always does, whole or not: range(0.0, 4.0) is a list of floats.
func rangeIsFloat(v any) bool {
	_, ok := phpval.Number(v).(float64)
	return ok
}

// rangeStepIsFloat is the same question for $step, where PHP is laxer: a step
// with no fractional part leaves an integer range integral, so range(0, 4, 2.0)
// is a list of ints while range(0, 10, 2.5) is a list of floats.
func rangeStepIsFloat(v any) bool {
	f, ok := phpval.Number(v).(float64)
	return ok && f != math.Trunc(f)
}

func rangeFloats(from, to, step float64) []any {
	if step == 0 {
		step = 1
	}
	span := math.Abs(to - from)
	n := int(math.Floor(span/step)) + 1
	out := make([]any, 0, n)
	for i := 0; i < n; i++ {
		if from <= to {
			out = append(out, from+float64(i)*step)
			continue
		}
		out = append(out, from-float64(i)*step)
	}
	return out
}

func rangeInts(from, to, step int64) []any {
	if step < 0 {
		step = -step
	}
	if step == 0 {
		step = 1
	}
	span := to - from
	if span < 0 {
		span = -span
	}
	n := int(span/step) + 1
	out := make([]any, 0, n)
	for i := 0; i < n; i++ {
		if from <= to {
			out = append(out, from+int64(i)*step)
			continue
		}
		out = append(out, from-int64(i)*step)
	}
	return out
}
