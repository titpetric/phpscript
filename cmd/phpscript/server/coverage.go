package server

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/titpetric/phpscript/internal/flags"
	"github.com/titpetric/phpscript/runner/coverage"
)

// CoveragePath names the endpoint where a running server publishes its counts.
//
// A profile on disk needs a writable filesystem, plus a shutdown. A test flow
// reads the numbers while the server is still up, right after the suite that
// produced them. The endpoint answers that without either.
const CoveragePath = "/debug/phpscript/coverage"

// coverageModule holds one server's process-lifetime coverage aggregator and its endpoint.
type coverageModule struct {
	ZeroModule
	aggregator *coverage.Aggregator
	globals    *flags.Options

	// roots are the source trees the counted files read from, a tree for each
	// site. A profile holds columns resolved from the source text. The server
	// knows where the text is.
	roots []fs.FS
}

// newCoverageModule returns the module a run with --cover registers.
func newCoverageModule(globals *flags.Options, roots ...fs.FS) *coverageModule {
	return &coverageModule{
		ZeroModule: *newZeroModule("phpcoverage"),
		aggregator: coverage.NewAggregatorMode(globals.CoverageMode()),
		globals:    globals,
		roots:      roots,
	}
}

// watch adds a site's source tree to the profile's column roots.
func (m *coverageModule) watch(root fs.FS) {
	m.roots = append(m.roots, root)
}

// Mount publishes the endpoint.
func (m *coverageModule) Mount(_ context.Context, r Router) error {
	r.Handle(CoveragePath, http.HandlerFunc(m.serve))
	return nil
}

// Stop writes the collected profile on the graceful shutdown path.
func (m *coverageModule) Stop(_ context.Context) error {
	if m.aggregator.Empty() {
		return nil
	}
	name, err := m.globals.WriteCoverProfile(m.blocks())
	if err != nil {
		return err
	}
	fmt.Printf("coverage: %.1f%% of statements, written to %s\n", coverage.Percent(m.blocks()), name)
	return nil
}

// serve answers the profile or the report ?mode= names, in --cover's vocabulary.
func (m *coverageModule) serve(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = coverage.ModeLine
	}
	blocks := m.blocks()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	switch mode {
	case coverage.ModeLine:
		_ = coverage.WriteProfile(w, blocks, m.aggregator.Mode())
	case coverage.ModeFunc:
		_ = coverage.WriteReport(w, coverage.FuncRows(blocks, m.aggregator.Functions(), m.aggregator.Files()))
	case coverage.ModeFile:
		_ = coverage.WriteReport(w, coverage.FileRows(blocks, m.aggregator.Files()))
	default:
		http.Error(w, fmt.Sprintf("unknown mode %q, want %s, %s or %s", mode, coverage.ModeLine, coverage.ModeFunc, coverage.ModeFile), http.StatusBadRequest)
	}
}

// blocks renders the counted statements with columns resolved from the source text.
func (m *coverageModule) blocks() []coverage.ProfileBlock {
	return coverage.Columns(m.aggregator.Blocks(), m.source)
}

// source reads a counted file from the site roots, nil when unreadable.
func (m *coverageModule) source(file string) []string {
	name := strings.TrimPrefix(path.Clean(file), "/")
	for _, root := range m.roots {
		data, err := fs.ReadFile(root, name)
		if err != nil {
			continue
		}
		return strings.Split(string(data), "\n")
	}
	return nil
}
