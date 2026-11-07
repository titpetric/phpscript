package model

import (
	"strings"
)

// This file maps a go import path onto the area that binds it. `import
// "net/http"` is the go spelling of `use Runtime\Go\Net\HTTP`. The mapping is
// mechanical: one path segment per area segment, where the name a script
// addresses the package by is the path's last segment. The parser reads it to
// resolve `http.NewRequest` while it parses. A binding package reads it to
// register under the name the import resolves to.

// goAreaWords spells the segments an area does not title-case. The areas under
// `Runtime\Go` carry go's own capitalisation of an acronym, and the mapping
// answers the name a folder of stdlib/go registers.
var goAreaWords = map[string]string{
	"html": "HTML",
	"http": "HTTP",
	"io":   "IO",
	"json": "JSON",
	"sql":  "SQL",
	"url":  "URL",
	"utf8": "UTF8",
	"xml":  "XML",
}

// GoImportArea maps a go import path onto the area that binds it and the name
// a script addresses that area by. It refuses a path that is no chain of go
// package names, a module path of a repository included: the import reaches
// the standard library alone.
func GoImportArea(path string) (area, alias string, ok bool) {
	segments := strings.Split(path, "/")
	parts := make([]string, 0, len(segments)+1)
	parts = append(parts, GoNamespace)
	for _, segment := range segments {
		if !goPackageName(segment) {
			return "", "", false
		}
		parts = append(parts, goAreaSegment(segment))
	}
	alias = segments[len(segments)-1]
	if goVersionSegment(alias) {
		return "", "", false
	}
	return strings.Join(parts, `\`), alias, true
}

// goAreaSegment is the area name one path segment takes.
func goAreaSegment(segment string) string {
	if word, ok := goAreaWords[segment]; ok {
		return word
	}
	return strings.ToUpper(segment[:1]) + segment[1:]
}

// goPackageName reports whether segment spells a go package: a lower-case
// letter, then letters, digits or underscores.
func goPackageName(segment string) bool {
	if segment == "" || segment[0] < 'a' || segment[0] > 'z' {
		return false
	}
	for i := 0; i < len(segment); i++ {
		c := segment[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '_':
		default:
			return false
		}
	}
	return true
}

// goVersionSegment reports whether segment is a major-version element, `v2` in
// `math/rand/v2`. It names no package, and a script addressing it would write
// `v2.Int()`.
func goVersionSegment(segment string) bool {
	if len(segment) < 2 || segment[0] != 'v' {
		return false
	}
	for i := 1; i < len(segment); i++ {
		if segment[i] < '0' || segment[i] > '9' {
			return false
		}
	}
	return true
}
