package apidoc

import (
	"reflect"
	"strings"
)

// goDocHost is the reference site a standard library citation links to.
const goDocHost = "https://pkg.go.dev/"

// Citation is one Go symbol the listing names: the package that declares it,
// the symbol, the reference page for a standard library symbol, and the
// spelling a script types when the runtime honours one.
type Citation struct {
	Package string `json:"package,omitempty"` // import path, empty for a builtin
	Name    string `json:"name,omitempty"`
	Go      string `json:"go,omitempty"`  // the selector Go source writes
	URL     string `json:"url,omitempty"` // reference page, empty outside the standard library
	PHP     string `json:"php,omitempty"` // the spelling a script types
}

// Symbol is the Go spelling of the citation, qualified by the import path.
func (c Citation) Symbol() string {
	if c.Package == "" {
		return c.Name
	}
	return c.Package + "." + c.Name
}

// Short is the selector Go source writes, Go holding the package name and the
// import path apart: encoding/json/v2 declares json.Marshaler.
func (c Citation) Short() string {
	if c.Go == "" {
		return c.Name
	}
	return c.Go
}

// Link renders the citation as markdown. A standard library symbol carries a
// link to its reference page; the listing names every other symbol bare.
func (c Citation) Link() string {
	if c.URL == "" {
		return c.Symbol()
	}
	return "[" + c.Symbol() + "](" + c.URL + ")"
}

// Clause is the name an implements clause carries: the PHP spelling when a
// script can type one, the Go spelling otherwise.
func (c Citation) Clause() string {
	if c.PHP != "" {
		return c.PHP
	}
	return c.Short()
}

// cite names the Go type t, with the reference page filled in for a type the
// standard library declares.
func cite(t reflect.Type) Citation {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	c := Citation{Package: t.PkgPath(), Name: t.Name(), Go: t.String()}
	switch {
	case isStdlib(c.Package):
		c.URL = goDocHost + c.Package + "#" + c.Name
	case c.Package == "" && c.Name == "error":
		// error is the one interface the language itself declares, and
		// builtin is where its page lives.
		c.URL = goDocHost + "builtin#error"
	}
	return c
}

// isStdlib reports whether an import path names a package of the Go standard
// library. The first element of such a path carries no dot, the test the go
// command applies to tell a standard library package from a module one.
func isStdlib(pkg string) bool {
	if pkg == "" {
		return false
	}
	first, _, _ := strings.Cut(pkg, "/")
	return !strings.Contains(first, ".")
}
