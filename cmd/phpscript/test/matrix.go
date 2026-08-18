package test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

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
type matrixRow struct {
	DisplayPath string
	Cells       []matrixCell
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
func runMatrix(ctx context.Context, fixtures []*tests.Fixture, displayPaths []string, opts Options) int {
	var table matrixTable
	if !opts.JSON {
		table = newMatrixTable(os.Stdout, displayPaths, opts)
		table.writeHeader()
	}

	var jsonRows []jsonFixture
	var passedCount, failedCount int
	startAll := time.Now()

	for i, fx := range fixtures {
		row := matrixRow{DisplayPath: displayPaths[i]}
		for _, name := range tests.Runners {
			cell := matrixCell{Runner: name}
			if !fx.Runs(name) {
				cell.Status = matrixSkip
				cell.Reason = fmt.Sprintf("opted out by runner.%s: false", name)
				row.Cells = append(row.Cells, cell)
				jsonRows = append(jsonRows, jsonFixture{
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
				jsonRows = append(jsonRows, jsonFixture{
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

		if row.Failed() {
			failedCount++
		} else {
			passedCount++
		}
		if table != nil {
			table.writeRow(row)
		}
	}

	if table != nil {
		table.close()
		table.writeSummary(passedCount, failedCount, len(fixtures), time.Since(startAll))
	}

	if opts.JSON {
		_ = json.NewEncoder(os.Stdout).Encode(jsonReport{
			Total:   len(fixtures),
			Passed:  passedCount,
			Failed:  failedCount,
			Results: jsonRows,
		})
	}

	return failedCount
}
