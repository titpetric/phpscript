package formatter

// SkipError reports a file the formatter left alone: source it cannot parse,
// a node it has no spelling for, or output that did not hold up to the checks
// in File. Formatting rewrites a file in place and the formatter leaves
// anything it does not fully understand untouched, which is safer than a
// rewrite from a partial reading.
type SkipError struct {
	Path   string
	Reason error
}

// Error names the file and the fault in it.
func (e *SkipError) Error() string { return e.Path + ": " + e.Reason.Error() }

// Unwrap answers the reason and a caller matches on the parse error itself.
func (e *SkipError) Unwrap() error { return e.Reason }
