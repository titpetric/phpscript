package formatter

import (
	"errors"
	"fmt"
	"os"

	"github.com/titpetric/phpscript/list"
	"github.com/titpetric/phpscript/parser"
)

// Paths formats each path argument in place and reports what happened to every
// file it looked at. It skips a file it cannot format and the run carries on:
// a directory of PHP holds valid code phpscript does not support yet, and one
// such file must not stop it formatting the rest. The error it returns covers
// reading and writing alone.
func Paths(paths []string) ([]Result, error) {
	return walk(paths, true)
}

// NeedFormatting reports what Paths would do, writing nothing.
func NeedFormatting(paths []string) ([]Result, error) {
	return walk(paths, false)
}

// Changed returns the paths of the rewritten files.
func Changed(results []Result) []string {
	var out []string
	for _, r := range results {
		if r.Changed {
			out = append(out, r.Path)
		}
	}
	return out
}

// walk formats every file paths names, writing each one back where write says
// so. Reading and writing errors end the walk; a file the formatter cannot
// take responsibility for comes back skipped and the walk carries on.
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
// returns a *SkipError for a file it left untouched.
func File(path string) (bool, error) {
	return format(path, true)
}

// format formats one file, writing it back where write says so, and reports
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
// reach the disk on the first.
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
