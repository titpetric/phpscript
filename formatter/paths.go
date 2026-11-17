package formatter

import (
	"errors"
	"fmt"
	"os"

	"github.com/titpetric/phpscript/list"
)

// Paths formats each path argument in place, skipping what it cannot.
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

// walk formats each file paths names, ending on a read error and skipping an unsupported file.
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

// File formats path in place, a *SkipError for one it left untouched.
func File(path string) (bool, error) {
	return format(path, true)
}

// format formats one file in place when write is true and reports whether it changed.
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

// verify refuses output that does not parse or does not survive a second format.
func verify(out string) error {
	if _, _, err := parseSource(out); err != nil {
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
