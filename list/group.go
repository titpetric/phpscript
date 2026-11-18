package list

import (
	"path/filepath"
)

// PathGroup is the paths of one folder: Dir holds the folder, "." for paths in
// the run directory. Indexes holds the positions of its paths in the input
// slice; a caller can keep per-path data alongside without the group copying it.
type PathGroup struct {
	Dir     string
	Indexes []int
}

// GroupByDir buckets paths by the folder holding them, preserving the order
// the input established, between folders and inside one. Bucketing is explicit,
// never read off the sort: sorted paths do not keep a folder contiguous.
// Paths a/b.php, a/m/x.php, a/z.php interleave.
func GroupByDir(paths []string) []PathGroup {
	index := map[string]int{}
	var groups []PathGroup

	for i, p := range paths {
		dir := filepath.ToSlash(filepath.Dir(p))
		at, ok := index[dir]
		if !ok {
			at = len(groups)
			index[dir] = at
			groups = append(groups, PathGroup{Dir: dir})
		}
		groups[at].Indexes = append(groups[at].Indexes, i)
	}

	return groups
}
