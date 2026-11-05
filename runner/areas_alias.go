package runner

import (
	"strings"
	"sync"
)

// areaMembers answers which function names an area defines, keyed by the
// folded area name. A host sets the source once; stdlib sets the one for its
// areas.
var areaMembers struct {
	sync.Mutex
	current *areaTable
}

// areaTable is the membership read from one source, built on first use.
type areaTable struct {
	source func() map[string][]string
	once   sync.Once
	byArea map[string]map[string]struct{}
}

// SetAreaMembers installs the source of area membership, read on the first qualified call.
func SetAreaMembers(source func() map[string][]string) {
	areaMembers.Lock()
	defer areaMembers.Unlock()
	areaMembers.current = nil
	if source != nil {
		areaMembers.current = &areaTable{source: source}
	}
}

// areaFunc resolves a name written through its area to the global name it
// stands for, and reports whether the area defines or replaces it.
func areaFunc(name string) (string, bool) {
	if !underArea(name) {
		return "", false
	}
	cut := strings.LastIndexByte(name, '\\')
	if cut <= 0 {
		return "", false
	}
	area, fn := foldName(name[:cut]), name[cut+1:]

	areaMembers.Lock()
	table := areaMembers.current
	areaMembers.Unlock()
	if table == nil {
		return "", false
	}
	table.once.Do(func() {
		table.byArea = map[string]map[string]struct{}{}
		for areaName, fns := range table.source() {
			set := make(map[string]struct{}, len(fns))
			for _, f := range fns {
				set[strings.ToLower(f)] = struct{}{}
			}
			table.byArea[foldName(areaName)] = set
		}
	})
	if _, ok := table.byArea[area][strings.ToLower(fn)]; !ok {
		return "", false
	}
	return fn, true
}

// underArea reports whether a name starts with Runtime\PS\ or Runtime\PX\,
// without allocating: every failed function lookup passes through here.
func underArea(name string) bool {
	n := len(NamespacePS)
	if len(name) > 0 && name[0] == '\\' {
		name = name[1:]
	}
	if len(name) <= n+1 || name[n] != '\\' {
		return false
	}
	head := name[:n]
	return strings.EqualFold(head, NamespacePS) || strings.EqualFold(head, NamespacePX)
}
