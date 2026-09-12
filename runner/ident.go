package runner

// The expression compiler keys variable slots by an identifier spelling that
// keeps the three per-evaluation namespaces apart: a PHP variable, a bare
// name (a constant), and a compiled closure literal (`__cl<N>`). The prefixes
// date from the transpiler, which shared one env namespace with function
// names and had to avoid collisions; the slot table keeps them because
// resolveVar still reads the kind off the identifier.

// varIdent is the identifier used for a PHP variable. `$this` stays `this`;
// everything else is prefixed.
func varIdent(name string) string {
	if name == "this" {
		return "this"
	}
	return "v_" + name
}

// constIdentPrefix marks the identifier of a bare name. The runtime reads it
// to decide what an unresolved name means.
const constIdentPrefix = "c_"

// constIdent is the identifier for a bare name, kept apart from the variable
// of the same spelling: `define("x", 1); echo x;` and `echo $x;` are two
// lookups, and only the second is null when nothing set it.
func constIdent(name string) string { return constIdentPrefix + name }
