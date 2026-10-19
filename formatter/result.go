package formatter

// Result is what the formatter did with one file. Formatting rewrites files
// in place, so the formatter rewrites one only where it could read the whole
// file and the output held up to the checks in File; every other file comes
// back skipped and untouched.
type Result struct {
	Path    string
	Changed bool
	// Skipped is the reason the file stays untouched, and nil for a formatted
	// file.
	Skipped error
}
