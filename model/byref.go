package model

// byRefArgs lists, per builtin, the argument positions PHP passes by reference.
// They are output parameters: the binding receives a setter that writes the
// caller's variable instead of a value. The positions are PHP's, counted from
// zero: preg_match_all($pattern, $subject, &$matches) and
// preg_replace_callback($pattern, $callback, $subject, $limit, &$count).
var byRefArgs = map[string]map[int]bool{
	"preg_match_all":        {2: true},
	"preg_match":            {2: true},
	"preg_replace_callback": {4: true},
	"parse_str":             {1: true},

	// The exit status of a command. exec's $output is not here: it is an
	// array, and an array is shared, so the binding appends into the one the
	// caller passed and reaches the same observable result. An integer has no
	// such route back.
	"exec":     {2: true},
	"system":   {1: true},
	"passthru": {1: true},
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
