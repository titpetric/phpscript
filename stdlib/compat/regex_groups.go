package compat

import (
	"github.com/titpetric/phpscript/internal/phpval"
)

// patternOrder fills $matches with one entry per capture group: $matches[0]
// holds every whole match and $matches[g] every capture of group g. The
// columns are present even when nothing matched, so a caller may index
// $matches[1] before it checks the count.
//
// Without a flag that changes the entries, the columns are []string. The VM
// indexes and iterates both levels exactly like nested PHP arrays, and the
// columns are plain string slices, so a match set of g groups over n matches
// costs g+1 allocations where an *model.Array pair would cost 2(g+1) plus 2n
// interface boxes. PREG_OFFSET_CAPTURE boxes every entry anyway and takes the
// []any branch.
func patternOrder(subject string, all [][]int, groups int, flags int64) []any {
	out := make([]any, 0, groups+1)
	boxed := flags&(pregOffsetCapture|pregUnmatchedAsNull) != 0
	for g := 0; g <= groups; g++ {
		if boxed {
			col := make([]any, len(all))
			for i, idx := range all {
				col[i] = matchValue(subject, idx, g, flags)
			}
			out = append(out, col)
			continue
		}
		col := make([]string, len(all))
		for i, idx := range all {
			col[i] = groupText(subject, idx, g)
		}
		out = append(out, col)
	}
	return out
}

// setOrder fills $matches with one entry per match, each holding the whole
// match followed by its groups: the transpose of patternOrder.
func setOrder(subject string, all [][]int, groups int, flags int64) []any {
	out := make([]any, len(all))
	for i, idx := range all {
		out[i] = matchGroups(subject, idx, groups, flags)
	}
	return out
}

// matchOffset resolves preg_match's $offset argument to a byte offset into
// subject. A negative offset counts from the end. An offset outside the
// subject is a failed call: PHP returns false, so the second result reports
// whether the offset is usable at all.
func matchOffset(subject string, offset any) (int, bool) {
	if offset == nil {
		return 0, true
	}
	start := int(phpval.Int(offset))
	if start < 0 {
		start += len(subject)
	}
	if start < 0 || start > len(subject) {
		return 0, false
	}
	return start, true
}

// splitPieces accumulates preg_split's result. It holds a []string while the
// flags allow it, for the same reason patternOrder does, and switches to []any
// only for PREG_SPLIT_OFFSET_CAPTURE, where every piece is a pair anyway.
type splitPieces struct {
	flags int64
	text  []string
	pairs []any
}

// add appends the piece of subject between the byte offsets start and end, and
// reports whether it landed in the result: PREG_SPLIT_NO_EMPTY drops an empty
// one. A start of -1 is an unmatched capture group, which is an empty piece
// at offset -1.
func (s *splitPieces) add(subject string, start, end int) bool {
	piece := ""
	if start >= 0 {
		piece = subject[start:end]
	}
	if s.flags&pregSplitNoEmpty != 0 && piece == "" {
		return false
	}
	if s.flags&pregSplitOffsetCapture != 0 {
		s.pairs = append(s.pairs, []any{piece, int64(start)})
		return true
	}
	s.text = append(s.text, piece)
	return true
}

// result returns the pieces in the shape the script sees. An empty result is
// an empty array and never null, as PHP returns where every piece drops.
func (s *splitPieces) result() any {
	if s.flags&pregSplitOffsetCapture != 0 {
		if s.pairs == nil {
			return []any{}
		}
		return s.pairs
	}
	if s.text == nil {
		return []string{}
	}
	return s.text
}

// matchGroups renders one match as the row a script reads: index 0 is the
// whole match and index g is group g.
//
// PHP drops the trailing groups that did not match, so count($m) is smaller
// for a match that ended before the last optional group;
// PREG_UNMATCHED_AS_NULL turns those into nulls instead and then they all
// stay. The flagless case is a []string for the reason patternOrder states.
func matchGroups(subject string, idx []int, groups int, flags int64) any {
	n := participatingGroups(idx, groups, flags)
	if flags&(pregOffsetCapture|pregUnmatchedAsNull) == 0 {
		row := make([]string, n)
		for g := 0; g < n; g++ {
			row[g] = groupText(subject, idx, g)
		}
		return row
	}
	row := make([]any, n)
	for g := 0; g < n; g++ {
		row[g] = matchValue(subject, idx, g, flags)
	}
	return row
}

// participatingGroups reports how many entries of a match row PHP keeps: every
// group up to the last one that participated, and always at least the whole
// match. PREG_UNMATCHED_AS_NULL keeps them all, since a null records the
// unmatched group and hides nothing.
func participatingGroups(idx []int, groups int, flags int64) int {
	if flags&pregUnmatchedAsNull != 0 {
		return groups + 1
	}
	n := groups + 1
	for n > 1 && groupStart(idx, n-1) < 0 {
		n--
	}
	return n
}

// groupStart returns the byte offset group g matched at, or -1 when the group
// did not match or the pattern has no such group.
func groupStart(idx []int, g int) int {
	if 2*g+1 >= len(idx) {
		return -1
	}
	return idx[2*g]
}

// groupText returns the text group g matched, empty for a group that did not
// match.
func groupText(subject string, idx []int, g int) string {
	start := groupStart(idx, g)
	if start < 0 {
		return ""
	}
	return subject[start:idx[2*g+1]]
}

// matchValue renders group g the way $matches holds it under flags:
// PREG_OFFSET_CAPTURE makes it a pair of the text and its byte offset, -1 for
// a group that did not match, and PREG_UNMATCHED_AS_NULL makes that group's
// text null where PHP writes empty.
func matchValue(subject string, idx []int, g int, flags int64) any {
	start := groupStart(idx, g)
	var text any = ""
	switch {
	case start >= 0:
		text = subject[start:idx[2*g+1]]
	case flags&pregUnmatchedAsNull != 0:
		text = nil
	}
	if flags&pregOffsetCapture != 0 {
		return []any{text, int64(start)}
	}
	return text
}

// writeRef assigns v to a by-reference out parameter. The setter is nil when
// the script left the argument off, which is legal for every one of them.
func writeRef(set func(any), v any) {
	if set != nil {
		set(v)
	}
}
