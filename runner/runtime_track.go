package runner

import (
	"sort"
)

// Track runs install and reports what it defined and replaced; it serves introspection.
func (rt *Runtime) Track(install func(*Runtime)) Delta {
	funcs := make(map[string]*funcEntry, len(rt.funcs))
	for name, entry := range rt.funcs {
		funcs[name] = entry
	}
	classes := map[string]struct{}{}
	for _, name := range rt.DeclaredClasses() {
		classes[name] = struct{}{}
	}
	ctors := make(map[string]*funcEntry, len(rt.constructors))
	for name, entry := range rt.constructors {
		ctors[name] = entry
	}
	constants := make(map[string]struct{}, len(rt.constants))
	for name := range rt.constants {
		constants[name] = struct{}{}
	}

	install(rt)

	var d Delta
	for name, entry := range rt.funcs {
		switch before, ok := funcs[name]; {
		case !ok:
			d.Funcs = append(d.Funcs, name)
		case before != entry:
			d.Replaced = append(d.Replaced, name)
		}
	}
	for _, name := range rt.DeclaredClasses() {
		if _, ok := classes[name]; !ok {
			d.Classes = append(d.Classes, name)
		}
	}
	for name := range rt.constants {
		if _, ok := constants[name]; !ok {
			d.Constants = append(d.Constants, name)
		}
	}
	for name, entry := range rt.constructors {
		if before, ok := ctors[name]; ok && before != entry {
			d.Replaced = append(d.Replaced, name)
		}
	}
	sort.Strings(d.Funcs)
	sort.Strings(d.Replaced)
	sort.Strings(d.Classes)
	sort.Strings(d.Constants)
	return d
}
