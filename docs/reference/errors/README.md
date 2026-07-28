# Errors

| PHP language-reference feature | Status | Notes |
| --- | --- | --- |
| Parse errors | Compatibility | Invalid source is rejected with a line-oriented parser error. |
| Runtime errors | Partial compatibility | Failures are represented as Go errors and can be caught by script code. |
| PHP error levels and handlers | Not implemented | `E_*`, `set_error_handler()`, and `trigger_error()` semantics are unavailable. |
| Error-control operator | Not implemented | `@` does not suppress failures. |
| Host error callback | phpscript extension | `Runtime.OnError` consumes statement errors after invoking the callback. |

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
