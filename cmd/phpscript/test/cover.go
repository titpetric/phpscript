package test

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/titpetric/phpscript/runner/coverage"
	"github.com/titpetric/phpscript/tests"
)

// DefaultCoverFile is where --cover writes the profile when --coverfile does
// not name one.
const DefaultCoverFile = "phpscript.cov"

// The --cover modes. Line is what a bare --cover means: measure, write the
// profile, print the one-line percentage. Func and file also write the profile,
// then print a per-symbol report in the format go tool cover -func prints.
const (
	CoverLine = "line"
	CoverFunc = "func"
	CoverFile = "file"
)

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
		return writeCoverReport(os.Stdout, funcRows(blocks, coverFuncs(fixtures), coverFiles(fixtures)))
	case CoverFile:
		return writeCoverReport(os.Stdout, fileRows(blocks, coverFiles(fixtures)))
	default:
		// The folder table is the fuller answer and it has already been
		// printed, so this line would only invite a comparison between two
		// numbers measuring different things: the profile counts the .phpt
		// entrypoints, which run by definition, and the table does not.
		if !opts.JSON && opts.Verbose {
			fmt.Printf("coverage: %.1f%% of statements\n", coveragePercent(blocks))
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
	sortProfile(blocks)
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

// coverRow is one line of a func or file report: the location column go tool
// cover -func prints, the symbol charged, and its statement-weighted percent.
type coverRow struct {
	File    string
	Line    int
	Name    string
	Covered int
	Total   int
}

// percent is the row's statement-weighted coverage, adjusted for the empty
// case: a symbol with no runnable statement has nothing left uncovered, so
// 0/0 reads as covered rather than as a zero dragging every average down.
func (r coverRow) percent() float64 {
	if r.Total == 0 {
		return 100
	}
	return float64(r.Covered) / float64(r.Total) * 100
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

// funcRows charges each profile block to the innermost declaration span
// containing its start line. A block inside no declaration is the file's
// top-level code, reported as {main} — the name PHP gives that scope — at the
// first such line.
func funcRows(blocks []profileBlock, funcs []coverage.FuncSpan, files []string) []coverRow {
	type key struct {
		file string
		line int
		name string
	}
	rows := map[key]*coverRow{}
	row := func(file string, line int, name string) *coverRow {
		k := key{file: file, line: line, name: name}
		r, ok := rows[k]
		if !ok {
			r = &coverRow{File: file, Line: line, Name: name}
			rows[k] = r
		}
		return r
	}
	for _, fn := range funcs {
		row(fn.File, fn.StartLine, fn.Name)
	}
	for _, b := range reportBlocks(blocks) {
		var at *coverage.FuncSpan
		for i := range funcs {
			fn := &funcs[i]
			if fn.File != b.File || b.StartLine < fn.StartLine || b.StartLine > fn.EndLine {
				continue
			}
			if at == nil || fn.EndLine-fn.StartLine < at.EndLine-at.StartLine {
				at = fn
			}
		}
		var r *coverRow
		if at != nil {
			r = row(at.File, at.StartLine, at.Name)
		} else {
			r = row(b.File, 1, "{main}")
		}
		r.Total += b.NumStmt
		if b.Count > 0 {
			r.Covered += b.NumStmt
		}
	}
	// A registered file none of whose declarations or top-level lines charged
	// anything holds nothing runnable; it reports as one adjusted {main} row
	// rather than disappearing.
	charged := map[string]bool{}
	for k := range rows {
		charged[k.file] = true
	}
	for _, file := range files {
		if !charged[file] {
			row(file, 1, "{main}")
		}
	}
	out := make([]coverRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, *r)
	}
	sortRows(out)
	return out
}

// fileRows reports one row per registered source file. A file with no
// runnable statement stays in the report at its adjusted percentage.
func fileRows(blocks []profileBlock, files []string) []coverRow {
	rows := map[string]*coverRow{}
	row := func(file string) *coverRow {
		r, ok := rows[file]
		if !ok {
			r = &coverRow{File: file, Line: 1, Name: path.Base(file)}
			rows[file] = r
		}
		return r
	}
	for _, file := range files {
		row(file)
	}
	for _, b := range reportBlocks(blocks) {
		r := row(b.File)
		r.Total += b.NumStmt
		if b.Count > 0 {
			r.Covered += b.NumStmt
		}
	}
	out := make([]coverRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, *r)
	}
	sortRows(out)
	return out
}

func sortRows(rows []coverRow) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].File != rows[j].File {
			return rows[i].File < rows[j].File
		}
		if rows[i].Line != rows[j].Line {
			return rows[i].Line < rows[j].Line
		}
		return rows[i].Name < rows[j].Name
	})
}

// writeCoverReport prints rows the way go tool cover -func does: location,
// symbol and percent columns, and a statement-weighted total line that
// downstream consumers recognize by name and skip.
func writeCoverReport(w io.Writer, rows []coverRow) error {
	tw := tabwriter.NewWriter(w, 0, 8, 1, '\t', 0)
	var covered, total int
	for _, r := range rows {
		covered += r.Covered
		total += r.Total
		fmt.Fprintf(tw, "%s:%d:\t%s\t%.1f%%\n", r.File, r.Line, r.Name, r.percent())
	}
	pct := 0.0
	if total > 0 {
		pct = float64(covered) / float64(total) * 100
	}
	fmt.Fprintf(tw, "total:\t(statements)\t%.1f%%\n", pct)
	return tw.Flush()
}
