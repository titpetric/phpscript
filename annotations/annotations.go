// Package annotations discovers annotated PHP files in a source tree and gives
// them a lifecycle: routed endpoints are served over HTTP, startup jobs run once
// before the server listens, and scheduled jobs run on an interval.
//
// Two annotations are recognised, both written as comments:
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
// The tag can be repeated to register multiple handlers. In the case of a
// duplicate handler being registered, the last one wins and a warning is
// printed in the logs.
//
// If method is omitted, only GET and POST are routed to the handler. This
// ignores requests like HEAD and OPTIONS, ideally leaving these to be resolved
// in the router, rather than invoking PHP.
//
// Specific HTTP methods like PUT are only reachable when explicitly stated.
//
// The extracted path parameters are merged into `$_REQUEST`, so a route
// declared `/users/{id}` answers `$_REQUEST['id']`. `$_PATH` is the name
// that merge replaced and stays seeded with the path values alone.
//
// It relies on the Go standard library to extract path parameters present.
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
// `N times per hour|day`. Calendar specs fire at local midnight. A tick is
// skipped when the previous run is still going. Output is recorded on the
// oida span as `output`.
//
// # Discovery
//
// The .php files are scanned recursively, so you may keep them in a single
// folder, or just place them in arbitrary subfolders. Files without annotations
// are skipped, as are `vendor` directories: a composer dependency does not get
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
