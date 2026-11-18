// Package annotations discovers annotated PHP files in a source tree and binds
// each file a lifecycle. A routed endpoint answers over HTTP. A startup job runs
// once before the server listens. A scheduled job runs on an interval.
//
// The scanner reads three tags, each written as a comment:
//
//   - `// @route GET /users/{id}` registers an HTTP endpoint.
//   - `// @startup` marks a file the server executes before it listens.
//   - `// @schedule daily -- prune` runs a file on a clock or interval.
//
// # Routes
//
// The `@route` tag takes an HTTP method, a path, optionally separated from the
// tag by a colon, one route per line:
//
//   - `// @route GET /users/{id}`
//   - `// @route POST /users/{id}`
//   - `// @route: /users/{id}`
//
// A file may repeat the tag to register more than one handler. Where two register
// the same handler, the last wins, and the logs record a warning.
//
// A tag naming no method routes GET and POST to the handler alone. Requests
// like HEAD and OPTIONS stay out; the router answers those itself. No PHP sees
// them.
//
// Specific HTTP methods like PUT are only reachable when explicitly stated.
//
// The handler merges the extracted path parameters into `$_REQUEST`; a route
// declared `/users/{id}` answers `$_REQUEST['id']`.
//
// The Go standard library does the extracting.
//
// # Startup jobs
//
// A file annotated `@startup` runs once, in path order, before the server
// listens. It executes with the CLI SAPI and can migrate a schema or warm a
// cache. An error aborts startup.
//
// # Scheduled jobs
//
// A file may repeat `@schedule` with an interval spec and optional `-- args`
// passed as `$argv`. Specs: `every N seconds|minutes|hours`, `hourly`, `daily`,
// `weekly`, `monthly`, `every weekday`, `every sunday` (any weekday name),
// `N times per hour|day`. Calendar specs fire at local midnight. The scheduler
// drops a tick while the previous run is still going, and records output on the
// oida span as `output`.
//
// # Discovery
//
// The scan reads .php files recursively. A tree may keep them in a single folder
// or spread them over subfolders. The scan skips a file holding no annotation,
// and skips a `vendor` directory. A composer dependency publishes no routes into
// the application and runs nothing at startup.
package annotations

import (
	"strings"
)

// comment returns a comment line's text, reporting whether a line holds one.
func comment(line string) (string, bool) {
	text := strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(text, "//"):
		text = strings.TrimPrefix(text, "//")
	case strings.HasPrefix(text, "#"):
		text = strings.TrimPrefix(text, "#")
	case strings.HasPrefix(text, "/*"):
		text = strings.TrimPrefix(text, "/*")
	case strings.HasPrefix(text, "*"):
		text = strings.TrimPrefix(text, "*")
	default:
		return "", false
	}
	text = strings.TrimSpace(text)
	text = strings.TrimSpace(strings.TrimSuffix(text, "*/"))
	return text, true
}

// tag splits a comment into annotation tag and fields, dropping a trailing colon.
func tag(text string) (string, []string) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "", nil
	}
	return strings.TrimSuffix(fields[0], ":"), fields[1:]
}
