package test

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/titpetric/phpscript/runner/coverage"
	"github.com/titpetric/phpscript/tests"
)

// DefaultCoverFile is where --cover writes the profile when --coverfile does
// not name one.
const DefaultCoverFile = "phpscript.cov"

// profileBlock is one line of a written profile: a coverage block with its
// columns resolved from the source text. The collector reports lines only; the
// columns exist so the profile is the format go test writes and go tool cover
// renders, spanning the statement text rather than the indentation around it.
type profileBlock struct {
	File      string
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
	NumStmt   int
	Count     int
}

// writeCoverage renders the collectors installed on the fixtures: one profile
// per fixture next to it when --split is set, and the merged profile at
// opts.CoverFile either way. It runs after the fixtures did, whichever mode ran
// them; a fixture whose runtime runner never executed simply has empty counts.
func writeCoverage(fixtures []*tests.Fixture, opts Options) error {
	merged := map[profileKey]*profileBlock{}
	for _, fx := range fixtures {
		if fx.Coverage() == nil {
			continue
		}
		blocks := fixtureCoverBlocks(fx)
		if opts.Split {
			split := strings.TrimSuffix(fx.Path, ".phpt") + ".cov"
			if err := writeCoverProfile(split, blocks); err != nil {
				return err
			}
		}
		for _, b := range blocks {
			key := profileKey{file: b.File, startLine: b.StartLine, endLine: b.EndLine}
			at, ok := merged[key]
			if !ok {
				block := b
				merged[key] = &block
				continue
			}
			// The same statement counted by several fixtures ran once per
			// fixture, so the counts add up; the shape of the block does not
			// change.
			at.Count += b.Count
			if b.NumStmt > at.NumStmt {
				at.NumStmt = b.NumStmt
			}
		}
	}

	blocks := make([]profileBlock, 0, len(merged))
	for _, b := range merged {
		blocks = append(blocks, *b)
	}
	sortProfile(blocks)
	if err := writeCoverProfile(opts.CoverFile, blocks); err != nil {
		return err
	}
	if !opts.JSON {
		fmt.Printf("coverage: %.1f%% of statements\n", coveragePercent(blocks))
	}
	return nil
}

// profileKey identifies a block across fixtures. Columns are derived from the
// same source lines everywhere, so the line range is the identity.
type profileKey struct {
	file      string
	startLine int
	endLine   int
}

// fixtureCoverBlocks renders one fixture's collected coverage: include paths
// are joined onto the fixture's include root, so the profile names files as
// they sit below the invocation directory, and the columns come from the
// source text.
func fixtureCoverBlocks(fx *tests.Fixture) []profileBlock {
	sources := map[string][]string{}
	lines := func(file string) []string {
		if cached, ok := sources[file]; ok {
			return cached
		}
		var src []string
		if file == fx.Path {
			// The entrypoint is the .phpt itself, but its lines count from the
			// start of the PHP section, which is what the parser saw.
			src = strings.Split(fx.PHP, "\n")
		} else if data, err := os.ReadFile(filepath.Join(fx.RootDir(), file)); err == nil {
			src = strings.Split(string(data), "\n")
		}
		sources[file] = src
		return src
	}

	var out []profileBlock
	for _, b := range fx.Coverage().Blocks() {
		block := profileBlock{
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
		if b.File != fx.Path {
			block.File = filepath.ToSlash(filepath.Join(fx.RootDir(), b.File))
		}
		out = append(out, block)
	}
	sortProfile(out)
	return out
}

func sortProfile(blocks []profileBlock) {
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

// writeCoverProfile writes blocks in the format go test -coverprofile writes,
// with the measurement mode go names "count".
func writeCoverProfile(path string, blocks []profileBlock) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create coverfile: %w", err)
	}
	defer f.Close()
	if _, err := fmt.Fprintln(f, "mode: count"); err != nil {
		return fmt.Errorf("write coverfile: %w", err)
	}
	for _, b := range blocks {
		_, err := fmt.Fprintf(f, "%s:%d.%d,%d.%d %d %d\n",
			b.File, b.StartLine, b.StartCol, b.EndLine, b.EndCol, b.NumStmt, b.Count)
		if err != nil {
			return fmt.Errorf("write coverfile: %w", err)
		}
	}
	return nil
}

// coveragePercent is the statement-weighted percentage go test reports:
// statements in blocks that ran, over all registered statements.
func coveragePercent(blocks []profileBlock) float64 {
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

// coverFixtures installs one collector per fixture. Per-fixture collectors are
// what --split reports from, and they keep parallel fixtures from sharing
// mutable state; the merge above reassembles the whole run.
func coverFixtures(fixtures []*tests.Fixture) {
	for _, fx := range fixtures {
		fx.SetCoverage(coverage.New())
	}
}
