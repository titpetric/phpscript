package main

import (
	"strings"

	"github.com/titpetric/phpscript/list"
	"github.com/titpetric/phpscript/runner"
)

// generator renders the inventory: the paths an import reaches, the standard
// library symbol table behind each package, the class metadata that names a
// wrapper's Go type.
type generator struct {
	paths    map[string]string
	packages map[string]*goPackage
	classes  map[string]classLink
}

// classLink is the target of a class row link: the Go type behind a wrapper,
// the Go name of each method it exposes.
type classLink struct {
	path    string
	name    string
	methods map[string]string
}

// newGenerator reads the metadata a link needs: the import path of every area,
// the Go type and method names behind each class.
func newGenerator() *generator {
	g := &generator{
		paths:    importPaths(),
		packages: map[string]*goPackage{},
		classes:  classLinks(),
	}
	return g
}

// classLinks reads the class listing of the go runtime, keyed by the name a
// script types.
func classLinks() map[string]classLink {
	out := map[string]classLink{}
	classes, err := list.StdlibClasses(runner.Options{Runtime: goRuntime})
	if err != nil {
		return out
	}
	for _, class := range classes {
		area, name := splitClassName(class.Name)
		link := classLink{methods: map[string]string{}}
		if class.Uses.URL != "" {
			link.path, link.name = class.Uses.Package, class.Uses.Name
		}
		for _, method := range class.Methods {
			goName := method.GoName
			if goName == "" {
				goName = method.Name
			}
			link.methods[method.Name] = goName
		}
		out[area+`\`+name] = link
	}
	return out
}

// splitClassName splits a class name into its area and short name.
func splitClassName(name string) (area, short string) {
	i := strings.LastIndex(name, `\`)
	if i < 0 {
		return "", name
	}
	return name[:i], name[i+1:]
}

// url answers the pkg.go.dev link for one row, and empty where the package
// declares no such symbol. The check reads the standard library source. A name
// the runtime adds next to a package, such as http.Aborted, stays plain.
func (g *generator) url(area, kind, name string) string {
	shortName := short(area, name)
	path := g.paths[area]

	switch kind {
	case "function", "constant":
		if g.holds(path, shortName) {
			return docURL(path, shortName)
		}
	case "class":
		link := g.classes[area+`\`+shortName]
		if g.holds(link.path, link.name) {
			return docURL(link.path, link.name)
		}
		if g.holds(path, shortName) {
			return docURL(path, shortName)
		}
	case "method":
		class, method, ok := strings.Cut(shortName, "::")
		if !ok {
			return ""
		}
		link := g.classes[area+`\`+class]
		recv, path := class, path
		if link.path != "" {
			recv, path = link.name, link.path
		}
		goName := link.methods[method]
		if goName == "" {
			goName = method
		}
		if g.hasMethod(path, recv, goName) {
			return docURL(path, recv+"."+goName)
		}
	}
	return ""
}

// holds reports the package declaring a top-level name. A path the source tree
// does not hold names nothing. The row stays plain.
func (g *generator) holds(path, name string) bool {
	if path == "" || name == "" {
		return false
	}
	pkg, ok := g.lookup(path)
	return ok && pkg.has(name)
}

// hasMethod reports the package declaring the method on the type.
func (g *generator) hasMethod(path, recv, method string) bool {
	if path == "" || recv == "" || method == "" {
		return false
	}
	pkg, ok := g.lookup(path)
	return ok && pkg.hasMethod(recv, method)
}

// lookup reads a package's symbol table once and keeps it.
func (g *generator) lookup(path string) (*goPackage, bool) {
	if pkg, ok := g.packages[path]; ok {
		return pkg, pkg != nil
	}
	pkg, err := loadPackage(path)
	if err != nil {
		pkg = nil
	}
	g.packages[path] = pkg
	return pkg, pkg != nil
}

// packageURL builds the reference page of a package.
func packageURL(path string) string {
	return "https://pkg.go.dev/" + path
}

// docURL builds the reference URL: the package page with the symbol's anchor,
// the name for a top-level symbol, Type.Method for a method.
func docURL(path, anchor string) string {
	return packageURL(path) + "#" + anchor
}

// named renders a name as a table cell, linked to its declaration.
func (g *generator) named(area, kind, name string) string {
	if url := g.url(area, kind, name); url != "" {
		return "[" + "`" + name + "`" + "](" + url + ")"
	}
	return "`" + name + "`"
}
