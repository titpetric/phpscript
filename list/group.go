package list

import "path/filepath"

// PathGroup is the paths of one folder: Dir holds the folder, "." when the
// paths sit in the run directory, and Indexes the positions of its paths in
// the input slice, so a caller can carry per-path data alongside without the
// group copying it.
type PathGroup struct {
	Dir     string
	Indexes []int
}

// GroupByDir buckets paths by the folder holding them, preserving the order
// the input established both between folders and inside one. Bucketing is
// explicit rather than relying on the sort, because sorted paths do not keep
// a folder contiguous: a/b.php, a/m/x.php and a/z.php interleave.
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
