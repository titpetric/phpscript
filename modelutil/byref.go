package modelutil

// byRefArgs names, per builtin, the argument positions PHP passes by reference.
var byRefArgs = map[string]map[int]bool{
	"preg_match_all":        {2: true},
	"preg_match":            {2: true},
	"preg_replace_callback": {4: true},
	"parse_str":             {1: true},

	// The exit status of a command. exec's $output is not here: the caller and
	// the binding share one array and the binding appends into the one the
	// caller passed and reaches the same observable result. An integer has no
	// such route back.
	"exec":     {2: true},
	"system":   {1: true},
	"passthru": {1: true},
}

// ByRefArg reports whether the argument at index is an output parameter of the
// named call. Either name may match; a namespaced call falls back to global.
func ByRefArg(name, fallback string, index int) bool {
	if refs, ok := byRefArgs[name]; ok {
		return refs[index]
	}
	return byRefArgs[fallback][index]
}
