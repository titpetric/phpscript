package runner

import (
	"fmt"
	"strings"
)

// NamespacePS is phpscript's own binding surface, which every build mounts. A
// script writes a name bare or under its area: strlen() is also
// \Runtime\PS\Core\Strings\strlen(), and the Database class is also
// \Runtime\PS\Database.
const NamespacePS = `Runtime\PS`

// NamespacePX is the overlay phpx installs over Runtime\PS. It holds the names
// phpx registers over phpscript's, and the ones phpscript leaves undefined.
const NamespacePX = `Runtime\PX`

// NamespaceGo is the opt-in binding of go's standard library. `use
// Runtime\Go\Strings` installs one area on the runtime that runs the file, and
// a runtime no file imported defines none of these names.
const NamespaceGo = `Runtime\Go`

// NamespaceNone selects no area at all: the runtime carries the language and the
// exception classes, which are not bindings, with the standard library's
// functions left out. It suits a plugin host whose scripts compute over values
// the host hands in.
const NamespaceNone = `Runtime\None`

// NamespaceUnnamed holds an installer contributed without a name.
const NamespaceUnnamed = NamespacePS + `\Unnamed`

// RegisterArea contributes an installer under ProfileSecure in the named area.
func RegisterArea(name string, installer func(*Runtime)) {
	RegisterProfileArea(name, ProfileSecure, installer)
}

// RegisterProfileArea contributes an installer in the named area under a profile.
func RegisterProfileArea(name string, profile Profile, installer func(*Runtime)) {
	if installer == nil {
		return
	}
	if name == "" {
		name = NamespaceUnnamed
	}
	bindings.Lock()
	defer bindings.Unlock()
	bindings.installers = append(bindings.installers, binding{name: name, layer: areaLayer(name), profile: profile, install: installer})
}

// AreasFor returns the areas whose profile overlaps the mask, Runtime\PS first.
func AreasFor(profile Profile) []Area {
	bindings.Lock()
	defer bindings.Unlock()
	areas := make([]Area, 0, len(bindings.installers))
	for layer := 0; layer <= areaLayers; layer++ {
		for _, b := range bindings.installers {
			if b.layer == layer && b.profile&profile != 0 {
				areas = append(areas, Area{Name: b.name, Profile: b.profile, Install: b.install})
			}
		}
	}
	return areas
}

// SelectAreas keeps the areas that sit under one of the namespaces. An empty
// list keeps them all: the default runtime carries every area, Runtime\PS
// first. Runtime\None keeps none, whatever else the list names.
func SelectAreas(areas []Area, namespaces []string) []Area {
	if len(namespaces) == 0 {
		return areas
	}
	for _, ns := range namespaces {
		if foldName(ns) == foldName(NamespaceNone) {
			return nil
		}
	}
	out := make([]Area, 0, len(areas))
	for _, area := range areas {
		for _, ns := range namespaces {
			if InNamespace(area.Name, ns) {
				out = append(out, area)
				break
			}
		}
	}
	return out
}

// InNamespace reports whether name is the namespace or sits below it, as php compares names.
func InNamespace(name, namespace string) bool {
	name, namespace = foldName(name), foldName(namespace)
	return name == namespace || strings.HasPrefix(name, namespace+`\`)
}

// areaLayers is the highest layer: 0 is Runtime\PS and the rest, 1 is Runtime\PX.
const areaLayers = 1

// areaLayer orders the areas: the overlay installs after what it overlays.
func areaLayer(name string) int {
	if InNamespace(name, NamespacePX) {
		return 1
	}
	return 0
}

// Areas reports the namespaces the runtime was built to install; empty means all.
func (rt *Runtime) Areas() []string { return rt.opts.Areas }

// CheckAreas refuses a namespace that names no registered area or opt-in
// namespace, since a misspelling would select nothing and leave a runtime
// without bindings. The three roots always pass, so a configuration valid in
// one build is valid in another, and a root that no linked area sits under
// selects nothing; a deeper name is checked against the areas linked into this
// one.
func CheckAreas(namespaces []string) error {
	registered := AreasFor(^Profile(0))
	opt := Namespaces()
	if len(registered) == 0 {
		return nil
	}
	for _, ns := range namespaces {
		found := isRoot(ns)
		for _, area := range registered {
			found = found || InNamespace(area.Name, ns)
		}
		for _, name := range opt {
			found = found || InNamespace(name, ns)
		}
		if !found {
			return fmt.Errorf("runner.areas: %q names no area; phpscript list --areas lists them", ns)
		}
	}
	return nil
}

// isRoot reports whether ns is Runtime\PS, Runtime\PX, Runtime\Go or
// Runtime\None, whichever areas a build links in.
func isRoot(ns string) bool {
	for _, root := range []string{NamespacePS, NamespacePX, NamespaceGo, NamespaceNone} {
		if foldName(ns) == foldName(root) {
			return true
		}
	}
	return false
}
