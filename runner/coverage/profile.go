package coverage

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// DefaultCoverFile is where --cover writes the profile when --coverfile does
// not name one.
const DefaultCoverFile = "phpscript.cov"

// The --cover modes. Line is what a bare --cover means: measure, write the
// profile, print the one-line percentage. Func and file also write the profile,
// then print a per-symbol report in the format go tool cover -func prints.
const (
	ModeLine = "line"
	ModeFunc = "func"
	ModeFile = "file"
)

// ProfileBlock is one line of a written profile: a coverage block with its
// columns resolved from the source text. The collector reports lines only; the
// columns exist so the profile is the format go test writes and go tool cover
// renders, spanning the statement text rather than the indentation around it.
type ProfileBlock struct {
	File      string
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
	NumStmt   int
	Count     int
}

// Columns resolves each block's columns against the source text of the file it
// came from. source returns the lines of a file, or nil when the host cannot
// read it, in which case the block keeps column 1 on both ends.
func Columns(blocks []Block, source func(file string) []string) []ProfileBlock {
	cache := map[string][]string{}
	lines := func(file string) []string {
		if cached, ok := cache[file]; ok {
			return cached
		}
		src := source(file)
		cache[file] = src
		return src
	}

	out := make([]ProfileBlock, 0, len(blocks))
	for _, b := range blocks {
		block := ProfileBlock{
			File:      b.File,
			StartLine: b.StartLine,
			StartCol:  1,
			EndLine:   b.EndLine,
			EndCol:    1,
			NumStmt:   b.NumStmt,
			Count:     b.Count,
		}
		if src := lines(b.File); src != nil {
			if b.StartLine >= 1 && b.StartLine <= len(src) {
				line := src[b.StartLine-1]
				block.StartCol = len(line) - len(strings.TrimLeft(line, " \t")) + 1
			}
			if b.EndLine >= 1 && b.EndLine <= len(src) {
				block.EndCol = len(src[b.EndLine-1]) + 1
			}
		}
		out = append(out, block)
	}
	SortProfile(out)
	return out
}

// SortProfile orders blocks by file and position, which is the order a profile
// is written in.
func SortProfile(blocks []ProfileBlock) {
	sort.Slice(blocks, func(i, j int) bool {
		if blocks[i].File != blocks[j].File {
			return blocks[i].File < blocks[j].File
		}
		if blocks[i].StartLine != blocks[j].StartLine {
			return blocks[i].StartLine < blocks[j].StartLine
		}
		return blocks[i].EndLine < blocks[j].EndLine
	})
}

// WriteProfile writes blocks in the format go test -coverprofile writes, with
// the measurement mode go names "count".
func WriteProfile(w io.Writer, blocks []ProfileBlock) error {
	if _, err := fmt.Fprintln(w, "mode: count"); err != nil {
		return fmt.Errorf("write coverfile: %w", err)
	}
	for _, b := range blocks {
		_, err := fmt.Fprintf(w, "%s:%d.%d,%d.%d %d %d\n",
			b.File, b.StartLine, b.StartCol, b.EndLine, b.EndCol, b.NumStmt, b.Count)
		if err != nil {
			return fmt.Errorf("write coverfile: %w", err)
		}
	}
	return nil
}

// Percent is the statement-weighted percentage go test reports: statements in
// blocks that ran, over all registered statements.
func Percent(blocks []ProfileBlock) float64 {
	var covered, total int
	for _, b := range blocks {
		total += b.NumStmt
		if b.Count > 0 {
			covered += b.NumStmt
		}
	}
	if total == 0 {
		return 0
	}
	return float64(covered) / float64(total) * 100
}
