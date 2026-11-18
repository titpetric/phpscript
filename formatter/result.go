package formatter

// Result reports the formatter's work on one file. Formatting rewrites files
// in place. The formatter rewrites a file only when it could read the whole
// file and the output passed the checks in File. Any other file comes back
// skipped and untouched.
type Result struct {
	Path    string
	Changed bool
	// Skipped is the reason the file stays untouched, and nil for a formatted
	// file.
	Skipped error
}
