package phpval

// AutoGlobals holds PHP's superglobals in fixed fields instead of map
// entries. The set is closed by the language, so the per-store "is this a
// superglobal" question - asked on every variable assignment in both
// engines - reduces to one byte compare and a switch instead of a map
// probe, and a request reset is eight field writes instead of a map clear.
//
// The fields are any, not a concrete array type: a script may assign
// whatever it likes over a superglobal, and phpscript keeps PHP's answer.
// No lock: a runtime serves one request on one goroutine, and the write
// path is the same single-threaded store path every other variable takes.
type AutoGlobals struct {
	Cookie  any
	Env     any
	Files   any
	Get     any
	Post    any
	Request any
	Server  any
	Session any
}

// field returns the storage for name, nil when name is not a superglobal.
// The first-byte check rejects almost every variable before the switch.
func (g *AutoGlobals) field(name string) *any {
	if len(name) < 4 || name[0] != '_' {
		return nil
	}
	switch name {
	case "_COOKIE":
		return &g.Cookie
	case "_ENV":
		return &g.Env
	case "_FILES":
		return &g.Files
	case "_GET":
		return &g.Get
	case "_POST":
		return &g.Post
	case "_REQUEST":
		return &g.Request
	case "_SERVER":
		return &g.Server
	case "_SESSION":
		return &g.Session
	}
	return nil
}

// Set claims a superglobal store, reporting whether it did.
func (g *AutoGlobals) Set(name string, value any) bool {
	f := g.field(name)
	if f == nil {
		return false
	}
	*f = value
	return true
}

// Lookup answers a read. The bool reports a bound superglobal: an unset one
// answers false so the caller keeps its fallback order (globals, constants).
func (g *AutoGlobals) Lookup(name string) (any, bool) {
	f := g.field(name)
	if f == nil || *f == nil {
		return nil, false
	}
	return *f, true
}

// Range visits every bound superglobal.
func (g *AutoGlobals) Range(visit func(name string, value any)) {
	for _, entry := range []struct {
		name  string
		value any
	}{
		{"_COOKIE", g.Cookie},
		{"_ENV", g.Env},
		{"_FILES", g.Files},
		{"_GET", g.Get},
		{"_POST", g.Post},
		{"_REQUEST", g.Request},
		{"_SERVER", g.Server},
		{"_SESSION", g.Session},
	} {
		if entry.value != nil {
			visit(entry.name, entry.value)
		}
	}
}

// Reset drops every binding; the next request's Register rebinds.
func (g *AutoGlobals) Reset() { *g = AutoGlobals{} }
