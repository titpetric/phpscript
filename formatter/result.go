package formatter

// Result is what the formatter did with one file. Formatting rewrites files
// in place, so a file is only rewritten when the formatter could read all of
// it and its output held up to the checks in File; every other file is
// reported as skipped and left as it is.
type Result struct {
	Path    string
	Changed bool
	// Skipped is the reason the file was left as it is, and nil when the file
	// was formatted.
	Skipped error
}
