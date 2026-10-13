package tests

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestWriteJSONReport covers the machine-readable report a run writes. The
// total is counted off the results, so a reader has the count without walking
// the list, and every TestResult field keeps the json name it was tagged with.
func TestWriteJSONReport(t *testing.T) {
	var out strings.Builder
	results := []*TestResult{
		{Name: "one", Path: "one.phpt", Passed: true, DurationMs: 3, Runner: RunnerRuntime},
		{Name: "two", Path: "two.phpt", FailureReason: "output differs"},
	}
	if err := WriteJSONReport(&out, results); err != nil {
		t.Fatal(err)
	}

	var report struct {
		Total   int           `json:"total"`
		Results []*TestResult `json:"results"`
	}
	if err := json.Unmarshal([]byte(out.String()), &report); err != nil {
		t.Fatalf("the report is not json: %v", err)
	}
	if report.Total != 2 {
		t.Errorf("total = %d, want 2", report.Total)
	}
	if !reflect.DeepEqual(report.Results, results) {
		t.Errorf("results = %+v, want %+v", report.Results, results)
	}
}

// TestWriteHTMLReport covers the human-readable report. The passed and failed
// counts are summed over the results, so the summary line reports the same
// numbers the table rows carry.
func TestWriteHTMLReport(t *testing.T) {
	var out strings.Builder
	results := []*TestResult{
		{Name: "one", Path: "one.phpt", Passed: true, DurationMs: 3},
		{Name: "two", Path: "two.phpt", FailureReason: "output differs"},
		{Name: "three", Path: "three.phpt", FailureReason: "want <script>alpha</script>"},
	}
	if err := WriteHTMLReport(&out, results); err != nil {
		t.Fatal(err)
	}

	html := out.String()
	for _, want := range []string{"Total: 3", "Passed: 1", "Failed: 2", "output differs", "one.phpt"} {
		if !strings.Contains(html, want) {
			t.Errorf("the report does not carry %q", want)
		}
	}
	// html/template escapes what a failure message carries, so a fixture whose
	// expected output is markup cannot write tags into the report.
	if strings.Contains(html, "<script>alpha") {
		t.Error("the report carries an unescaped tag out of a failure message")
	}
	if !strings.Contains(html, "&lt;script&gt;alpha") {
		t.Error("the report does not carry the escaped failure message")
	}
}
