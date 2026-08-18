package formatter

import (
	"errors"
	"fmt"
	"os"

	"github.com/titpetric/phpscript/list"
	"github.com/titpetric/phpscript/parser"
)

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

// SkipError reports a file the formatter left alone: source it cannot parse,
// a node it has no spelling for, or output that did not hold up to the checks
// in File. Formatting rewrites a file in place, so anything the formatter does
// not fully understand is safer left as it is than rewritten from a partial
// reading of it.
type SkipError struct {
	Path   string
	Reason error
}

func (e *SkipError) Error() string { return e.Path + ": " + e.Reason.Error() }

func (e *SkipError) Unwrap() error { return e.Reason }

// Paths formats each path argument in place and reports what happened to every
// file it looked at. A file it cannot format is skipped rather than failing the
// run: a directory of PHP holds valid code phpscript does not support yet, and
// one such file should not stop the rest from being formatted. Only reading
// and writing errors are returned.
func Paths(paths []string) ([]Result, error) {
	return walk(paths, true)
}

// NeedFormatting reports what Paths would do, writing nothing.
func NeedFormatting(paths []string) ([]Result, error) {
	return walk(paths, false)
}

// Changed returns the paths of the files that were rewritten.
func Changed(results []Result) []string {
	var out []string
	for _, r := range results {
		if r.Changed {
			out = append(out, r.Path)
		}
	}
	return out
}

// walk formats every file selected by paths, writing each one back when write
// is set. Reading and writing errors end the walk; a file the formatter cannot
// take responsibility for is recorded as skipped and the walk continues.
func walk(paths []string, write bool) ([]Result, error) {
	files, err := list.ExpandFiles(paths)
	if err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(files))
	for _, file := range files {
		changed, err := format(file, write)
		skip := &SkipError{}
		switch {
		case errors.As(err, &skip):
			results = append(results, Result{Path: file, Skipped: err})
		case err != nil:
			return results, err
		default:
			results = append(results, Result{Path: file, Changed: changed})
		}
	}
	return results, nil
}

// File formats path in place. Reports whether the file contents changed, and
// returns a *SkipError for a file left as it is.
func File(path string) (bool, error) {
	return format(path, true)
}

// format formats one file, writing it back when write is set, and reports
// whether its contents changed.
func format(path string, write bool) (bool, error) {
	in, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	out, err := Source(string(in))
	if err != nil {
		return false, &SkipError{Path: path, Reason: err}
	}
	if out == string(in) {
		return false, nil
	}
	if err := verify(out); err != nil {
		return false, &SkipError{Path: path, Reason: err}
	}
	if !write {
		return true, nil
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// verify checks the two properties formatted output has to hold before it
// replaces code that works: it parses, and formatting it again does not change
// it. A printer defect that only shows up on the second pass would otherwise
// be written to disk on the first.
func verify(out string) error {
	if _, err := parser.Parse(out); err != nil {
		return fmt.Errorf("formatted output does not parse: %w", err)
	}
	again, err := Source(out)
	if err != nil {
		return fmt.Errorf("formatted output cannot be formatted again: %w", err)
	}
	if again != out {
		return errors.New("formatting is not stable: a second pass changes the output")
	}
	return nil
}
