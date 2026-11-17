package runner

import (
	"strings"

	"github.com/titpetric/phpscript/parser"
)

// classSource holds PHP source a host ships, declared on autoload's first lookup of one of its classes.
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

// declareClassSource declares the source holding class, reporting whether class exists after.
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
