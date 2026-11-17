package runner

// The expression compiler keys variable slots by an identifier spelling that
// keeps the three per-evaluation namespaces apart: a PHP variable, a bare
// name (a constant), and a compiled closure literal (`__cl<N>`). The prefixes
// date from the transpiler, which shared one env namespace with function
// names and had to avoid collisions; the slot table keeps them because
// resolveVar still reads the kind off the identifier.

// varIdent is the slot identifier for a PHP variable, $this staying this.
func varIdent(name string) string {
	if name == "this" {
		return "this"
	}
	return "v_" + name
}

// constIdentPrefix marks a bare name's slot identifier, which resolveVar reads.
const constIdentPrefix = "c_"

// constIdent is a bare name's identifier, kept apart from the variable's.
func constIdent(name string) string { return constIdentPrefix + name }
