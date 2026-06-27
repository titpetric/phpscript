# Error handling

There's significant differences between an error in phpscript and an
error in PHP. Since phpscript is a go runtime, the statements we're
usually dealing with here are in the following form:

```go
storage, err := NewStorage()
if err != nil {
	return err
}

res1, err := storage.Call(ctx, id)
if err != nil {
	return err
}
// ...
```

PHP simplifies this to the following syntax:

```php
$storage = new Storage;
$res1 = $storage->call($_GET['id']);
```

The VM provides implicit error handling of Go code. If an error is
returned from the Go binding, an `Exception` object is thrown in the VM.
You can handle the exception either in PHP or in Go.

## Handling PHP exceptions

There are a few important statements to handle exceptions in PHP:

- `try`
- `catch`
- `finally`
- `throw`

To create an exception in PHP, you would do:

```php
throw new Exception("Not found", 404);
```

The second parameter is an optional `code`, which can mean a HTTP Status
code, or custom error handling codes for your own errors. To catch
errors thrown from either Go or PHP code, you use a try/catch statement:

```php
try {
	$storage = new Storage;
	$storage->get($_GET['id']);
} catch ($e) {
	echo "An exception occured:\n\n";
	echo "Code: " . $e->getCode();
        echo "Message: " . $e->getMessage();
}
```

Certain fallback situations can be created with `finally`.

```php
try {
	// exception throwing code
} catch ($e) {
	// exception handling code
} finally {
	// code that always executes
}
```

The caught exception is also an `error` type, so it can be passed along
to Go functions like `func(err error)` from PHP code.

## Handing exceptions in Go

If an exception is unhandled in Go, a runtime callback can be registered
to catch and observe the exception.

```go
rt.OnError(func(err error) {
	telemetry.ObserveError(err)
})
```

The runtime context is accessible over `rt.Context()`.

## References

- [PHP Compatibility](../php-compatibility.md)
- [Runner.OnError](https://pkg.go.dev/github.com/titpetric/phpscript@main/runner#Runtime.OnError)
