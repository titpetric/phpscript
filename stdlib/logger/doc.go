// Package logger writes a log line and fails the span the work runs in when
// that line reports an error.
//
// It serves the steps a host runs on behalf of a script and does not want in
// the script's output. Migrations are the case it covers: a migration
// set runs at startup, where the script's output is an HTTP body or a rendered
// page. A per-file status line written there lands in the script's output.
//
//	log := logger.New(ctx, "migrate")
//	for _, item := range applied {
//		log.Info("migration", "file", item.Filename, "status", item.Status)
//	}
//
// The method set is Info, Warn and Error, spelled and behaving as slog spells
// the set. A library taking a logger declares the same shape.
// Output goes to slog.Default, read per call so a process that configures
// logging past a logger's construction still reaches it, or to the logger passed to
// WithLogger.
//
// The trace is not a second log. The logger records nothing on it per message
// and opens no spans for log output; Error alone reaches it, as the error of
// the span in the logger's context, which fails that span and the trace with
// it. This failure shows red on the front end and needs no line read.
// The span is not ended, because the caller that opened it ends it.
//
// Recording is nil safe, as the rest of the instrumentation is. A context
// without a trace leaves the span nil; its methods do nothing. A CLI run pays
// only for the log line it wanted anyway.
package logger
