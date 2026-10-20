// Package annotations discovers annotated PHP files in a source tree and binds
// them a lifecycle: a routed endpoint answers over HTTP, a startup job runs
// once before the server listens, and a scheduled job runs on an interval.
//
// The scanner reads three tags, each written as a comment:
//
//   - `// @route GET /users/{id}` registers an HTTP endpoint.
//   - `// @startup` marks a file the server executes before it listens.
//   - `// @schedule daily -- prune` runs a file on a clock or interval.
//
// # Routes
//
// The `@route` tag takes an HTTP method and a path, optionally separated from
// the tag by a colon, one route per line:
//
//   - `// @route GET /users/{id}`
//   - `// @route POST /users/{id}`
//   - `// @route: /users/{id}`
//
// A file may repeat the tag to register two or more handlers. Where two
// register the same handler, the last one wins and the logs carry a warning.
//
// A tag naming no method routes GET and POST to the handler alone. That leaves
// out requests like HEAD and OPTIONS, which the router answers on its own and
// no PHP sees.
//
// Specific HTTP methods like PUT are only reachable when explicitly stated.
//
// The handler merges the extracted path parameters into `$_REQUEST`, so a
// route declared `/users/{id}` answers `$_REQUEST['id']`.
//
// The Go standard library does the extracting.
//
// # Startup jobs
//
// A file carrying `@startup` runs once, in path order, before the server
// listens. It executes with the CLI SAPI, so it can migrate a schema or warm a
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
// The scan reads .php files recursively, so a tree may keep them in a single
// folder or spread them over subfolders. The scan passes over a file with no
// annotation, and over a `vendor` directory: a composer dependency does not get
// to publish routes into the application, or to run at startup.
package annotations

import (
	"strings"
)

// comment returns the text of a single-line or docblock comment, reporting
// whether line is a comment at all. Both annotations must appear in a comment,
// so a PHP string holding what looks like an annotation stays inert.
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

// tag splits a comment into its annotation tag and the fields that follow it.
// The tag may carry a trailing colon, which is not part of the tag name.
func tag(text string) (string, []string) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "", nil
	}
	return strings.TrimSuffix(fields[0], ":"), fields[1:]
}
