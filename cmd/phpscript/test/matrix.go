package test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/titpetric/phpscript/internal/table"
	"github.com/titpetric/phpscript/tests"
)

// matrixStatus is the outcome of one runner for one fixture.
type matrixStatus int

const (
	matrixPass matrixStatus = iota
	matrixFail
	matrixSkip
)

// matrixCell holds one runner's outcome and the detail printed under the row
// when the run is verbose.
type matrixCell struct {
	Runner tests.Runner
	Status matrixStatus
	Reason string
}

// matrixRow is a fixture and its cell per runner, in tests.Runners order.
// Metrics are the default runtime's: a row has one cost column and three
// runners, so the matrix compares correctness across runners and cost on the
// runtime the other two are measured against. Per-runner cost stays in --json.
type matrixRow struct {
	DisplayPath string
	Label       string
	Cells       []matrixCell
	Metrics     fixtureMetrics
}

type matrixFixtureResult struct {
	Row      matrixRow
	JSONRows []jsonFixture
}

// label returns what a table cell should show for the row.
func (r matrixRow) label() string {
	if r.Label != "" {
		return r.Label
	}
	return r.DisplayPath
}

// Failed reports whether any runner failed the fixture.
func (r matrixRow) Failed() bool {
	for _, cell := range r.Cells {
		if cell.Status == matrixFail {
			return true
		}
	}
	return false
}

// runMatrix runs every fixture through every runner and reports the number of
// failed fixtures. A fixture opted out of a runner, or a runner missing from
// the machine, is skipped rather than failed.
func runMatrix(ctx context.Context, groups []fixtureGroup, opts Options, report io.Writer) int {
	var sinks teeMatrixTable
	if !opts.JSON {
		sinks = append(sinks, newMatrixTable(os.Stdout, opts, !table.IsTerminal(os.Stdout)))
	}
	if report != nil {
		sinks = append(sinks, newMarkdownMatrix(report, opts))
	}

	var jsonRows []jsonFixture
	var passedCount, failedCount, total int
	startAll := time.Now()

	for _, group := range groups {
		sinks.writeGroup(group.Dir, group.Labels)
		groupPassed, groupFailed := 0, 0
		groupStart := time.Now()

		results := mapFixtures(group.Fixtures, opts.Parallel, func(i int, fx *tests.Fixture) matrixFixtureResult {
			row := matrixRow{DisplayPath: group.Paths[i], Label: group.Labels[i]}
			var fixtureJSONRows []jsonFixture
			for _, name := range tests.Runners {
				cell := matrixCell{Runner: name}
				if !fx.Runs(name) {
					cell.Status = matrixSkip
					cell.Reason = fmt.Sprintf("opted out by runner.%s: false", name)
					row.Cells = append(row.Cells, cell)
					fixtureJSONRows = append(fixtureJSONRows, jsonFixture{
						Name:    fx.Name,
						Path:    row.DisplayPath,
						Runner:  string(name),
						Skipped: true,
						Failure: cell.Reason,
					})
					continue
				}

				for _, fr := range runFixtureSamples(ctx, fx, name, opts) {
					fr.DisplayPath = row.DisplayPath
					fr.Label = row.Label
					if name == tests.RunnerRuntime {
						row.Metrics = fr.fixtureMetrics
					}
					fixtureJSONRows = append(fixtureJSONRows, jsonFixture{
						Name:        fx.Name,
						Path:        fr.DisplayPath,
						Runner:      string(name),
						Passed:      fr.Result.Passed,
						Skipped:     fr.Result.Skipped,
						Runs:        fr.Runs,
						DurationNs:  fr.Total.Nanoseconds(),
						P50Ns:       fr.P50.Nanoseconds(),
						P95Ns:       fr.P95.Nanoseconds(),
						P99Ns:       fr.P99.Nanoseconds(),
						AllocsPerOp: fr.AllocsPerOp,
						BytesPerOp:  fr.BytesPerOp,
						GCRuns:      fr.GCRuns,
						Failure:     fr.Result.FailureReason,
					})

					// A failing sample decides the cell; samples of one fixture
					// only differ when the runtime is not deterministic.
					if cell.Status == matrixPass && !fr.Result.Passed {
						cell.Status = matrixFail
						cell.Reason = fr.Result.FailureReason
						if fr.Result.Skipped {
							cell.Status = matrixSkip
						}
					}
				}
				row.Cells = append(row.Cells, cell)
			}
			return matrixFixtureResult{Row: row, JSONRows: fixtureJSONRows}
		})

		for _, result := range results {
			row := result.Row
			jsonRows = append(jsonRows, result.JSONRows...)
			if row.Failed() {
				groupFailed++
			} else {
				groupPassed++
			}
			sinks.writeRow(row)
		}

		sinks.closeGroup(groupTotals{
			Dir:      group.Dir,
			Passed:   groupPassed,
			Failed:   groupFailed,
			Total:    len(group.Fixtures),
			Duration: time.Since(groupStart),
		})
		passedCount += groupPassed
		failedCount += groupFailed
		total += len(group.Fixtures)
	}

	sinks.writeSummary(passedCount, failedCount, total, time.Since(startAll))

	if opts.JSON {
		_ = json.NewEncoder(os.Stdout).Encode(jsonReport{
			Total:   total,
			Passed:  passedCount,
			Failed:  failedCount,
			Results: jsonRows,
		})
	}

	return failedCount
}
