package formatter

// SkipError reports a file the formatter left alone: source it cannot parse,
// a node it has no spelling for, or output that did not hold up to the checks
// in File. Formatting rewrites a file in place, so anything the formatter does
// not fully understand is safer left as it is than rewritten from a partial
// reading of it.
type SkipError struct {
	Path   string
	Reason error
}

// Error names the file and what was wrong with it.
func (e *SkipError) Error() string { return e.Path + ": " + e.Reason.Error() }

// Unwrap answers the reason, so a caller matches on the parse error itself.
func (e *SkipError) Unwrap() error { return e.Reason }
