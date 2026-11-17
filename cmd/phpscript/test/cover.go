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
// command's own code reads and the two spell the same thing.
const (
	DefaultCoverFile = coverage.DefaultCoverFile
	CoverLine        = coverage.ModeLine
	CoverFunc        = coverage.ModeFunc
	CoverFile        = coverage.ModeFile
)

// profileBlock is one line of a written profile.
type profileBlock = coverage.ProfileBlock

// writeCoverage renders one profile per fixture under --split and the merged profile at opts.CoverFile.
func writeCoverage(opts Options, fixtures []*tests.Fixture) error {
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
		// The folder table is the fuller answer and the run printed it
		// already and this line would only invite a comparison between two
		// numbers measuring different things: the profile counts the .phpt
		// entrypoints, which run by definition; the table does not.
		if !opts.JSON && opts.Verbose {
			fmt.Printf("coverage: %.1f%% of statements\n", coverage.Percent(blocks))
		}
	}
	return nil
}

// mergeCoverBlocks folds every fixture's blocks into one profile, adding the counts of a shared statement.
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
			// A statement more than one fixture counted ran once per fixture,
			// and the counts add up; the shape of the block does not change.
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

// profileKey identifies a block across fixtures by its file and line range.
type profileKey struct {
	file      string
	startLine int
	endLine   int
}

// coverFilePath resolves a collected filename to the fixture path when it exists, the bare path otherwise.
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

// fixtureCoverBlocks renders one fixture's coverage with include paths resolved as the union served them.
func fixtureCoverBlocks(fx *tests.Fixture) []profileBlock {
	sources := map[string][]string{}
	lines := func(file string) []string {
		if cached, ok := sources[file]; ok {
			return cached
		}
		var src []string
		if file == fx.Path {
			// The entrypoint is the .phpt itself, but its lines count from the
			// start of the PHP section, the offset the parser saw.
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
		// filesystem's root. A profile names files by their path below the
		// invocation directory and the root comes back off here.
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

// coverFixtures installs one collector per fixture, keeping parallel fixtures off shared state.
func coverFixtures(fixtures []*tests.Fixture) {
	for _, fx := range fixtures {
		fx.SetCoverage(coverage.New())
	}
}

// coverFuncs merges every fixture's declaration spans, resolved to the profile's paths, entrypoints excluded.
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

// coverFiles merges every fixture's registered files into the report's file universe, entrypoints excluded.
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

// reportBlocks drops .phpt entrypoint blocks, whose top-level code runs by definition.
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
