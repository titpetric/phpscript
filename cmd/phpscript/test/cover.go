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

// The coverage vocabulary is the runner's; these names are what the test
// command's own code reads, so the two spell the same thing.
const (
	DefaultCoverFile = coverage.DefaultCoverFile
	CoverLine        = coverage.ModeLine
	CoverFunc        = coverage.ModeFunc
	CoverFile        = coverage.ModeFile
)

// profileBlock is one line of a written profile.
type profileBlock = coverage.ProfileBlock

// writeCoverage renders the collectors installed on the fixtures: one profile
// per fixture next to it when --split is set, and the merged profile at
// opts.CoverFile either way. It runs after the fixtures did, whichever mode ran
// them; a fixture whose runtime runner never executed simply has empty counts.
func writeCoverage(fixtures []*tests.Fixture, opts Options) error {
	if opts.Split {
		for _, fx := range fixtures {
			if fx.Coverage() == nil {
				continue
			}
			split := strings.TrimSuffix(fx.Path, ".phpt") + ".cov"
			if err := writeCoverProfile(split, fixtureCoverBlocks(fx)); err != nil {
				return err
			}
		}
	}

	blocks := mergeCoverBlocks(fixtures)
	if err := writeCoverProfile(opts.CoverFile, blocks); err != nil {
		return err
	}
	switch opts.Cover {
	case CoverFunc:
		return coverage.WriteReport(os.Stdout, coverage.FuncRows(reportBlocks(blocks), coverFuncs(fixtures), coverFiles(fixtures)))
	case CoverFile:
		return coverage.WriteReport(os.Stdout, coverage.FileRows(reportBlocks(blocks), coverFiles(fixtures)))
	default:
		// The folder table is the fuller answer and it has already been
		// printed, so this line would only invite a comparison between two
		// numbers measuring different things: the profile counts the .phpt
		// entrypoints, which run by definition, and the table does not.
		if !opts.JSON && opts.Verbose {
			fmt.Printf("coverage: %.1f%% of statements\n", coverage.Percent(blocks))
		}
	}
	return nil
}

// mergeCoverBlocks folds every fixture's collected blocks into one profile.
// The same statement reached by several fixtures is one block whose counts add
// up, which is what makes a folder's total independent of how its fixtures are
// split up.
func mergeCoverBlocks(fixtures []*tests.Fixture) []profileBlock {
	merged := map[profileKey]*profileBlock{}
	for _, fx := range fixtures {
		if fx.Coverage() == nil {
			continue
		}
		for _, b := range fixtureCoverBlocks(fx) {
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
	coverage.SortProfile(blocks)
	return blocks
}

// profileKey identifies a block across fixtures. Columns are derived from the
// same source lines everywhere, so the line range is the identity.
type profileKey struct {
	file      string
	startLine int
	endLine   int
}

// coverFilePath resolves a collected filename to the path the runtime read it
// from, as seen from the invocation directory. Includes resolve through a
// union: the fixture's directory answers first, the application root — the
// invocation directory itself — after it. The profile must name the file the
// union served, so the same order decides here: the fixture-joined path when
// it exists, the bare path otherwise.
func coverFilePath(fx *tests.Fixture, file string) string {
	joined := filepath.ToSlash(filepath.Join(fx.RootDir(), file))
	if _, err := os.Stat(filepath.FromSlash(joined)); err == nil {
		return joined
	}
	if _, err := os.Stat(filepath.FromSlash(file)); err == nil {
		return filepath.ToSlash(file)
	}
	return joined
}

// fixtureCoverBlocks renders one fixture's collected coverage: include paths
// resolve the way the fixture's include union resolved them, so the profile
// names files as they sit below the invocation directory, and the columns come
// from the source text.
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
		} else if data, err := os.ReadFile(filepath.FromSlash(coverFilePath(fx, file))); err == nil {
			src = strings.Split(string(data), "\n")
		}
		sources[file] = src
		return src
	}

	var out []profileBlock
	for _, b := range fx.Coverage().Blocks() {
		// The collector keys a file the way a script reads it, from the source
		// filesystem's root. A profile names files as they sit below the
		// invocation directory, so the root comes back off here.
		name := strings.TrimPrefix(b.File, "/")
		block := profileBlock{
			File:      name,
			StartLine: b.StartLine,
			StartCol:  1,
			EndLine:   b.EndLine,
			EndCol:    1,
			NumStmt:   b.NumStmt,
			Count:     b.Count,
		}
		if src := lines(name); src != nil {
			if b.StartLine >= 1 && b.StartLine <= len(src) {
				line := src[b.StartLine-1]
				block.StartCol = len(line) - len(strings.TrimLeft(line, " \t")) + 1
			}
			if b.EndLine >= 1 && b.EndLine <= len(src) {
				block.EndCol = len(src[b.EndLine-1]) + 1
			}
		}
		if name != fx.Path {
			block.File = coverFilePath(fx, name)
		}
		out = append(out, block)
	}
	coverage.SortProfile(out)
	return out
}

// writeCoverProfile writes blocks in the format go test -coverprofile writes.
func writeCoverProfile(path string, blocks []profileBlock) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create coverfile: %w", err)
	}
	defer f.Close()
	return coverage.WriteProfile(f, blocks)
}

// coverFixtures installs one collector per fixture. Per-fixture collectors are
// what --split reports from, and they keep parallel fixtures from sharing
// mutable state; the merge above reassembles the whole run.
func coverFixtures(fixtures []*tests.Fixture) {
	for _, fx := range fixtures {
		fx.SetCoverage(coverage.New())
	}
}

// coverFuncs merges every fixture's declaration spans, resolved to the paths
// the profile names. Fixture entrypoints do not declare application functions
// worth reporting, and their rows are excluded wholesale below, so their spans
// are dropped here too.
func coverFuncs(fixtures []*tests.Fixture) []coverage.FuncSpan {
	type key struct {
		file, name string
		start      int
	}
	merged := map[key]coverage.FuncSpan{}
	for _, fx := range fixtures {
		if fx.Coverage() == nil {
			continue
		}
		for _, fn := range fx.Coverage().Functions() {
			// As in fixtureCoverBlocks: the collector names a file from the
			// source filesystem's root, a report names it below the invocation
			// directory.
			fn.File = strings.TrimPrefix(fn.File, "/")
			if fn.File == fx.Path {
				continue
			}
			fn.File = coverFilePath(fx, fn.File)
			k := key{file: fn.File, name: fn.Name, start: fn.StartLine}
			if _, ok := merged[k]; !ok {
				merged[k] = fn
			}
		}
	}
	funcs := make([]coverage.FuncSpan, 0, len(merged))
	for _, fn := range merged {
		funcs = append(funcs, fn)
	}
	sort.Slice(funcs, func(i, j int) bool {
		if funcs[i].File != funcs[j].File {
			return funcs[i].File < funcs[j].File
		}
		return funcs[i].StartLine < funcs[j].StartLine
	})
	return funcs
}

// coverFiles merges every fixture's registered files, resolved to the paths
// the profile names, fixture entrypoints excluded. This is the report's file
// universe: a file of pure declarations carries no profile block, and only the
// register record says it was loaded at all.
func coverFiles(fixtures []*tests.Fixture) []string {
	seen := map[string]bool{}
	for _, fx := range fixtures {
		if fx.Coverage() == nil {
			continue
		}
		for _, file := range fx.Coverage().Files() {
			// As in fixtureCoverBlocks: the collector names a file from the
			// source filesystem's root, a report names it below the invocation
			// directory.
			file = strings.TrimPrefix(file, "/")
			if file == fx.Path {
				continue
			}
			seen[coverFilePath(fx, file)] = true
		}
	}
	files := make([]string, 0, len(seen))
	for f := range seen {
		files = append(files, f)
	}
	sort.Strings(files)
	return files
}

// reportBlocks filters the merged profile down to what a report charges:
// application sources. A .phpt entrypoint is the test itself — its top-level
// code runs by definition and would only inflate every summary built on the
// report.
func reportBlocks(blocks []profileBlock) []profileBlock {
	out := make([]profileBlock, 0, len(blocks))
	for _, b := range blocks {
		if strings.HasSuffix(b.File, ".phpt") {
			continue
		}
		out = append(out, b)
	}
	return out
}
