// Package logger wraps a slog logger for libraries that require one, and fails
// the span the work runs in when that logger is told about an error.
//
// Migrations are the case it was written for. mig reports per file status
// through a logger it now requires, and a migration set runs at startup, where
// the script's output is an HTTP body or a rendered page: a status line written
// there lands in what the script is producing.
//
//	options := migrate.NewOptions(logger.New(ctx, "migrate"))
//
// The method set is the one mig's migrate.Logger asks for, Info and Error,
// spelled the way slog spells them, and both are logged the way slog logs them.
// Output goes to slog.Default, read per call so a process that configures
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
