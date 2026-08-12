# Bindings

What makes the PHP runtime usable are the go bindings to the language.
You can create Go types like the runtime itself provides the `Exception`
object that is used when `new Exception` is evaluated in the VM.

The definition of an `Exception` on the Go side is as follows:

```go
package stdlib

// Exception holds an error code and error message.
type Exception struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// NewException will return a new allocation for an exception.
// A nil error is returned so a new exception is not thrown immediately
// whenever a `new Exception` call is made in the runtime.
func NewException(message string, code int) (Exception, error) {
	return Exception{
		Message: message,
		Code:    code,
	}, nil
}
```

All the methods bound to the type are passed along to the VM. The PHP VM
allows omitting arguments from the right side, leaving their default or
zero value when invoking the code.

```
rt := runtime.New(os.Stdout)
rt.RegisterConstructor("Exception", NewException)
```

An `Exception` type also implements `error`. The `(T, error)` returns
are implicitly handled by the VM to raise an exception. The constructor
always returns a nil error, so we can differentiate from a value (T) and
a thrown error (error). When creating an exception, you can use the
value as any other type, and invoke `getCode` and `getMessage`.

```php
$ex1 = new Exception("Not found", 404);
$ex2 = new Exception("Internal server error");
```

The latter example for `ex2` will produce the `0` code (zero value for
the argument). The functionality in comparison with PHP exceptions is
restricted but serves as a good example of how to create a Go binding.

## Shared memory

The PHP runtime is request driven. The `stdlib/ps` package provides a
concurrency-safe `SharedMemory` binding for retaining process-local state
between runtimes.

- `ps.NewSharedMemory() *ps.SharedMemory`
- `func (m *SharedMemory) Set(_ context.Context, key, value string)`
- `func (m *SharedMemory) Get(_ context.Context, key string) string`

Two utility functions bind the same instance to multiple runtimes:

- `ps.SharedMemoryContext(context.Context, *ps.SharedMemory) context.Context`
- `ps.NewSharedMemoryBinding(ctx context.Context) (*ps.SharedMemory, error)`

Register the standard-library constructor as follows:

```go
shm := ps.NewSharedMemory()
rt.SetContext(ps.SharedMemoryContext(rt.Context(), shm))
ps.RegisterSharedMemory(rt)
```

The bindings can now be used from PHP. Using the `@route` hints, a
request handler can look like this:

```
<?php

// @route POST /kv/{key}

$shm = new PS\SharedMemory;
$shm->incr("requests");
$shm->incr("post");
$shm->set($_PATH["key"], $_POST["value"]);

echo "ok";
```

## References

- [PHP exception class](https://www.php.net/manual/en/class.exception.php)
- [stdlib/Exception type](../../stdlib/exception.go)
- [stdlib/ps SharedMemory type](../../stdlib/ps/shared_memory.go)
- [Request routing](routing.md)
