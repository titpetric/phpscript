package runner

import (
	"strings"

	"github.com/titpetric/phpscript/parser"
)

// classSource is PHP source a host ships as standard-library classes, declared
// ahead of the registered loaders on autoload's first lookup of one of them. A
// script that never names one pays for none. file is the name the source
// reports and first is the class whose presence says the source is already in.
type classSource struct {
	file    string
	source  string
	first   string
	classes map[string]bool
}

// RegisterClassSource declares source on autoload's first lookup of a class in it.
func (rt *Runtime) RegisterClassSource(file, source string, classes ...string) {
	if len(classes) == 0 {
		return
	}
	names := make(map[string]bool, len(classes))
	for _, c := range classes {
		names[strings.ToLower(strings.TrimPrefix(c, "\\"))] = true
	}
	rt.classSources = append(rt.classSources, classSource{file: file, source: source, first: classes[0], classes: names})
}

// declareClassSource declares the source that holds class, and reports whether
// class exists once it has.
func (rt *Runtime) declareClassSource(class string) bool {
	key := strings.ToLower(strings.TrimPrefix(class, "\\"))
	for _, src := range rt.classSources {
		if !src.classes[key] {
			continue
		}
		if !rt.hasClass(src.first) && !rt.hasInterface(src.first) {
			prog, err := parser.ParseFile(src.file, src.source)
			if err != nil {
				return false
			}
			if err := rt.hoist(prog, src.file); err != nil {
				return false
			}
		}
		return rt.hasClass(class) || rt.hasInterface(class)
	}
	return false
}

func (rt *Runtime) hasInterface(name string) bool {
	_, ok := rt.lookupInterface(name)
	return ok
}
