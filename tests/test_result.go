package tests

// TestResult carries execution outcome for a single fixture.
type TestResult struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	Path          string `json:"path"`
	Passed        bool   `json:"passed"`
	DurationMs    int64  `json:"duration_ms"`
	FailureReason string `json:"failure_reason,omitempty"`
	GotOutput     string `json:"got_output,omitempty"`
	WantOutput    string `json:"want_output,omitempty"`
	Error         string `json:"error,omitempty"`
	Runner        Runner `json:"runner,omitempty"`
	Skipped       bool   `json:"skipped,omitempty"`
}
