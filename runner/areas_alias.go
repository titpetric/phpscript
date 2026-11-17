package runner

import (
	"strings"
	"sync"
)

// areaMembers holds the source of area function names, read once on first use.
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

// areaFunc resolves an area-qualified function name to the global name it names.
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

// areaClasses holds the source of area class aliases, read once on first use.
var areaClasses struct {
	sync.Mutex
	current *classTable
}

// classTable is the alias map built from one source on first use.
type classTable struct {
	source  func() map[string][]string
	once    sync.Once
	byAlias map[string]string
}

// SetAreaClasses installs the source of the classes each area installs, read on
// the first qualified lookup.
func SetAreaClasses(source func() map[string][]string) {
	areaClasses.Lock()
	defer areaClasses.Unlock()
	areaClasses.current = nil
	if source != nil {
		areaClasses.current = &classTable{source: source}
	}
}

// AreaName is the name a class answers to under the area that installs it:
// \Runtime\PS\Database for Database and \Runtime\PS\Database\Migrate for
// Database\Migrate. docs/reference/extensions/runtime-areas.md has the rule.
func AreaName(area, class string) string {
	// Exact, not folded: Runtime\PX\Datetime would resolve to the class DateTime.
	leaf := area
	if cut := strings.LastIndexByte(area, '\\'); cut >= 0 {
		leaf = area[cut+1:]
	}

	name := strings.TrimPrefix(class, nameRoot)
	if name == leaf {
		return area
	}

	return area + `\` + strings.TrimPrefix(name, leaf+`\`)
}

// nameRoot is the namespace prefix of every area, which a class name repeats.
const nameRoot = `Runtime\`

// areaClass resolves an area-qualified class name to the name it registered under.
func areaClass(name string) (string, bool) {
	if !underArea(name) {
		return "", false
	}

	areaClasses.Lock()
	table := areaClasses.current
	areaClasses.Unlock()
	if table == nil {
		return "", false
	}

	table.once.Do(func() {
		table.byAlias = map[string]string{}
		for area, classes := range table.source() {
			for _, class := range classes {
				alias := foldName(AreaName(area, class))
				if _, taken := table.byAlias[alias]; !taken {
					table.byAlias[alias] = class
				}
			}
		}
	})

	class, ok := table.byAlias[foldName(name)]
	return class, ok
}

// underArea reports whether name starts with Runtime\PS\ or Runtime\PX\, without allocating.
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
