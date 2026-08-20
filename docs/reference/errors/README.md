# Errors

| PHP language-reference feature | Status                | Notes                                                                          |
|--------------------------------|-----------------------|--------------------------------------------------------------------------------|
| Parse errors                   | Compatibility         | Invalid source is rejected with a line-oriented parser error.                  |
| Runtime errors                 | Partial compatibility | Failures are represented as Go errors and can be caught by script code.        |
| PHP error levels and handlers  | Not implemented       | `E_*`, `set_error_handler()`, and `trigger_error()` semantics are unavailable. |
| Error-control operator         | Not implemented       | `@` does not suppress failures.                                                |
| Host error callback            | phpscript extension   | `Runtime.OnError` consumes statement errors after invoking the callback.       |
| Errors from outside the script | phpscript extension   | `Runtime.RecordError` reports a failure no `try`/`catch` can reach.            |

phpscript uses one error path for explicit `throw` statements and errors
returned by Go-backed functions. It does not model PHP notices, warnings,
recoverable errors, or configurable reporting levels.

## Handling errors

Use `try` and `catch` for failures that can be recovered in script code. An
uncaught failure is returned to the Go host. See [Exceptions](../exceptions/README.md).

Installing `Runtime.OnError` changes that propagation: each non-exit statement
error is passed to the callback and execution continues. Because the error is
consumed at that point, the callback can prevent an enclosing script
`try`/`catch` from seeing it.

## Errors a script cannot catch

Some failures belong to the request rather than to the script: a body larger
than `post_max_size`, a file part larger than `upload_max_filesize`. They
happen before the first statement runs, so there is no statement to wrap in
`try`/`catch` and nothing is thrown. All a script sees is the result: an empty
`$_POST` and `$_FILES`, or an `UPLOAD_ERR_INI_SIZE` entry in `$_FILES`.

A Go host sees the reason. `Runtime.RecordError` is where they arrive, and it
does two things with each: records it on the trace of the request, where it
shows up in the debug front end under a `php error` span, and passes it to a
handler installed with `Runtime.OnError`. `Context.Errors` holds the same list,
for a host that would rather answer the request itself, with a 413 say, than
run the script at all.

```go
rt.OnError(func(err error) { log.Printf("request: %v", err) })
request := runner.FromRequestOptions(r, options) // records what the body got wrong
request.Register(rt)                             // reports it through rt.RecordError
```
