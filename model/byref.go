package model

// byRefArgs lists, per builtin, the argument positions PHP passes by reference.
// They are output parameters: the binding receives a setter that writes the
// caller's variable instead of a value. minitpl only needs preg_match_all's
// $matches, and preg_match's for the same reason.
var byRefArgs = map[string]map[int]bool{
	"preg_match_all": {2: true},
	"preg_match":     {2: true},
}

// ByRefArg reports whether the argument at index is an output parameter of the
// named call. Inside a namespaced file the call carries a qualified name and
// the global name it falls back to, and either may match.
func ByRefArg(name, fallback string, index int) bool {
	if refs, ok := byRefArgs[name]; ok {
		return refs[index]
	}
	return byRefArgs[fallback][index]
}
