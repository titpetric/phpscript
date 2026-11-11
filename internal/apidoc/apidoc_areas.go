package apidoc

import (
	"fmt"
	"sort"
	"strings"
)

// Owners takes the names of functions and of classes to the area that defines
// them, one table each because the two may share a name.
type Owners struct {
	Funcs, Classes map[string]string
}

// annotateNamespaces files each function and class under the area that
// defines it. A name no owner claims keeps an empty namespace and renders under
// "other".
func annotateNamespaces(funcs []Func, classes []Class, owners Owners) {
	for i := range funcs {
		funcs[i].Namespace = owners.Funcs[funcs[i].Name]
	}
	for i := range classes {
		classes[i].Namespace = owners.Classes[classes[i].Name]
	}
}

// hasNamespaces reports whether the functions name the area that defines them.
func hasNamespaces(funcs []Func) bool {
	for _, f := range funcs {
		if f.Namespace != "" {
			return true
		}
	}
	return false
}

// areaGroup is the heading an area files under: the first three segments of its
// namespace. Runtime\PS\Core\Strings\Compare files under Runtime\PS\Core.
func areaGroup(namespace string) string {
	if namespace == "" {
		return "other"
	}
	parts := strings.Split(namespace, `\`)
	if len(parts) > 3 {
		parts = parts[:3]
	}
	return strings.Join(parts, `\`)
}

// renderAreas writes the functions grouped by the area that installs them: a
// heading per group, and a subheading per area when the group holds more than
// one.
func renderAreas(b *strings.Builder, funcs []Func) {
	groups := map[string]map[string][]Func{}
	for _, f := range funcs {
		group := areaGroup(f.Namespace)
		if groups[group] == nil {
			groups[group] = map[string][]Func{}
		}
		groups[group][f.Namespace] = append(groups[group][f.Namespace], f)
	}
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, group := range names {
		fmt.Fprintf(b, "### %s\n\n", group)
		areas := make([]string, 0, len(groups[group]))
		for area := range groups[group] {
			areas = append(areas, area)
		}
		sort.Strings(areas)
		if len(areas) == 1 && (areas[0] == group || areas[0] == "") {
			renderFuncs(b, groups[group][areas[0]])
			continue
		}
		for _, area := range areas {
			title := area
			if title == "" {
				title = "other"
			}
			fmt.Fprintf(b, "#### %s\n\n", title)
			renderFuncs(b, groups[group][area])
		}
	}
}
