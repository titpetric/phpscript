package compat

import (
	"github.com/titpetric/phpscript/internal/phpval"
)

// patternOrder fills $matches by group, indexing $matches[g] across every match.
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

// setOrder fills $matches by match, each holding the whole match followed by its groups.
func setOrder(subject string, all [][]int, groups int, flags int64) []any {
	out := make([]any, len(all))
	for i, idx := range all {
		out[i] = matchGroups(subject, idx, groups, flags)
	}
	return out
}

// matchOffset resolves preg_match's $offset into bytes, reporting false when it falls outside subject.
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

// splitPieces accumulates preg_split's result, as strings until PREG_SPLIT_OFFSET_CAPTURE takes pairs.
type splitPieces struct {
	flags int64
	text  []string
	pairs []any
}

// add appends the piece between start and end, dropping an empty one under PREG_SPLIT_NO_EMPTY.
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

// result returns the pieces in the shape the script sees, an empty array when the split found nothing.
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

// matchGroups renders one match as a row, index 0 the whole match and index g group g.
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

// participatingGroups counts the entries PHP keeps, dropping trailing groups that did not match.
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

// groupStart returns the byte offset group g matched at, or -1 when it did not.
func groupStart(idx []int, g int) int {
	if 2*g+1 >= len(idx) {
		return -1
	}
	return idx[2*g]
}

// groupText returns the text group g matched, empty for a group that did not.
func groupText(subject string, idx []int, g int) string {
	start := groupStart(idx, g)
	if start < 0 {
		return ""
	}
	return subject[start:idx[2*g+1]]
}

// matchValue renders group g for $matches, a text-offset pair under PREG_OFFSET_CAPTURE.
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

// writeRef assigns v through set, and does nothing when the caller omits the argument.
func writeRef(set func(any), v any) {
	if set != nil {
		set(v)
	}
}
