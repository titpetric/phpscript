// Package logger writes a log line and fails the span the work runs in when
// that line reports an error.
//
// It is for the steps a host runs on behalf of a script and does not want in
// the script's output. Migrations are the case it was written for: a migration
// set runs at startup, where the script's output is an HTTP body or a rendered
// page, so a per-file status line written there lands in what the script is
// producing.
//
//	log := logger.New(ctx, "migrate")
//	for _, item := range applied {
//		log.Info("migration", "file", item.Filename, "status", item.Status)
//	}
//
// The method set is Info and Error, spelled and behaving the way slog spells
// them, which is also the shape a library asking to be handed a logger asks
// for. Output goes to slog.Default, read per call so a process that configures
// logging after a logger was built still gets it, or to the logger given to
// WithLogger.
//
// The trace is not a second log. Nothing is recorded on it per message, and no
// spans are opened for log output; Error alone reaches it, as the error of the
// span in the logger's context, which fails that span and the trace with it. A
// migration that failed is then red on the front end rather than a line someone
// has to read. The span is not ended, because the caller that opened it is the
// one that ends it.
//
// Recording is nil safe the way the rest of the instrumentation is: without a
// trace in the context the span is nil and its methods do nothing, so a CLI run
// pays nothing beyond the log line it wanted anyway.
package logger
