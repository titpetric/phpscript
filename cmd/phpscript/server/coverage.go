package server

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/titpetric/platform"

	"github.com/titpetric/phpscript/internal/flags"
	"github.com/titpetric/phpscript/runner/coverage"
)

// CoveragePath is where a running server publishes what it has counted so far.
//
// A profile on disk needs a writable filesystem and a shutdown; a test flow
// wants the numbers while the server is still up, right after the suite that
// produced them. The endpoint answers that without either.
const CoveragePath = "/debug/phpscript/coverage"

// coverageModule is the process-lifetime coverage of one server: the
// aggregator every request folds its counts into, the endpoint that publishes
// them, and the flush that writes the profile when the platform stops.
//
// A collector counts statements by AST node, so it belongs to one run; the
// aggregator holds one entry per statement range and is what survives the
// request that produced it. See runner/coverage.
type coverageModule struct {
	platform.UnimplementedModule
	aggregator *coverage.Aggregator
	globals    *flags.Options

	// roots are the source trees the counted files were read from, one per
	// site. A profile carries columns resolved from the source text, and the
	// server is what knows where the text is.
	roots []fs.FS
}

// newCoverageModule returns the module registered when --cover is set.
func newCoverageModule(globals *flags.Options, roots ...fs.FS) *coverageModule {
	return &coverageModule{
		UnimplementedModule: *platform.NewUnimplementedModule("phpcoverage"),
		aggregator:          coverage.NewAggregator(),
		globals:             globals,
		roots:               roots,
	}
}

// watch adds a site's source tree to the roots a profile resolves columns
// against. Every site registers one, and a virtual host server has several.
func (m *coverageModule) watch(root fs.FS) {
	m.roots = append(m.roots, root)
}

// Mount publishes the endpoint.
func (m *coverageModule) Mount(_ context.Context, r platform.Router) error {
	r.Handle(CoveragePath, http.HandlerFunc(m.serve))
	return nil
}

// Stop writes the profile the process collected. It runs on the graceful
// shutdown path, which is the only moment a server knows it is finished.
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

// serve answers with the profile, or with the report mode named by ?mode=. The
// vocabulary is --cover's: line is the profile itself, func and file are the
// per-symbol report in the format go tool cover -func prints.
func (m *coverageModule) serve(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = coverage.ModeLine
	}
	blocks := m.blocks()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	switch mode {
	case coverage.ModeLine:
		_ = coverage.WriteProfile(w, blocks)
	case coverage.ModeFunc:
		_ = coverage.WriteReport(w, coverage.FuncRows(blocks, m.aggregator.Functions(), m.aggregator.Files()))
	case coverage.ModeFile:
		_ = coverage.WriteReport(w, coverage.FileRows(blocks, m.aggregator.Files()))
	default:
		http.Error(w, fmt.Sprintf("unknown mode %q, want %s, %s or %s", mode, coverage.ModeLine, coverage.ModeFunc, coverage.ModeFile), http.StatusBadRequest)
	}
}

// blocks renders what has been counted, with the columns a profile carries
// resolved from the source text.
func (m *coverageModule) blocks() []coverage.ProfileBlock {
	return coverage.Columns(m.aggregator.Blocks(), m.source)
}

// source reads one counted file out of the site roots. A file no root answers
// for keeps column 1 on both ends rather than dropping out of the profile.
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
