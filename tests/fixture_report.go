package tests

import (
	"encoding/json"
	"html/template"
	"io"
)

// WriteJSONReport outputs a JSON report of the test results.
func WriteJSONReport(w io.Writer, results []*TestResult) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(map[string]any{
		"total":   len(results),
		"results": results,
	})
}

var htmlReportTemplate = template.Must(template.New("report").Parse(`<!DOCTYPE html>
<html>
<head>
	<meta charset="utf-8">
	<title>PHPScript Test Results</title>
	<style>
		body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; margin: 2rem; background: #0f172a; color: #f8fafc; }
		h1 { margin-bottom: 0.5rem; }
		.summary { font-size: 1.1rem; margin-bottom: 2rem; }
		.pass-count { color: #4ade80; font-weight: bold; }
		.fail-count { color: #f87171; font-weight: bold; }
		table { width: 100%; border-collapse: collapse; background: #1e293b; border-radius: 8px; overflow: hidden; }
		th, td { padding: 12px 16px; text-align: left; border-bottom: 1px solid #334155; }
		th { background: #334155; color: #94a3b8; text-transform: uppercase; font-size: 0.8rem; letter-spacing: 0.05em; }
		.status { font-weight: bold; padding: 4px 8px; border-radius: 4px; display: inline-block; font-size: 0.85rem; }
		.status.PASS { background: #166534; color: #4ade80; }
		.status.FAIL { background: #991b1b; color: #f87171; }
		.details { font-family: monospace; font-size: 0.9rem; color: #cbd5e1; white-space: pre-wrap; margin-top: 4px; }
	</style>
</head>
<body>
	<h1>PHPScript Test Results</h1>
	<div class="summary">
		Total: {{.Total}} | <span class="pass-count">Passed: {{.Passed}}</span> | <span class="fail-count">Failed: {{.Failed}}</span>
	</div>
	<table>
		<thead>
			<tr>
				<th>Status</th>
				<th>Name</th>
				<th>Path</th>
				<th>Duration</th>
				<th>Details</th>
			</tr>
		</thead>
		<tbody>
			{{range .Results}}
			<tr>
				<td><span class="status {{if .Passed}}PASS{{else}}FAIL{{end}}">{{if .Passed}}PASS{{else}}FAIL{{end}}</span></td>
				<td><strong>{{.Name}}</strong><br><small style="color:#94a3b8">{{.Description}}</small></td>
				<td><code>{{.Path}}</code></td>
				<td>{{.DurationMs}} ms</td>
				<td>{{if .FailureReason}}<div class="details">{{.FailureReason}}</div>{{else}}n/a{{end}}</td>
			</tr>
			{{end}}
		</tbody>
	</table>
</body>
</html>`))

// WriteHTMLReport outputs an HTML report of the test results.
func WriteHTMLReport(w io.Writer, results []*TestResult) error {
	var passed, failed int
	for _, r := range results {
		if r.Passed {
			passed++
		} else {
			failed++
		}
	}

	return htmlReportTemplate.Execute(w, map[string]any{
		"Total":   len(results),
		"Passed":  passed,
		"Failed":  failed,
		"Results": results,
	})
}
