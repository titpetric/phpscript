# Usage

## Go API bridge

The phpscript runner allows to inject Go types into the PHP VM and then
use them. The methods automatically get the request context, and any
error returned is explicitly handled and thrown. To give you a basic
idea from a passing test fixture:

```php
<?php

$storage = new Storage;
$storage->set("greeting", "hello");
$storage->set("name", "world");

$greeting = $storage->get("greeting");
$name = $storage->get("name");
$count = $storage->len();
```

In effect the above becomes:

```go
storage, err := NewStorage(ctx)
if err != nil {
	return err
}
if err := storage.Set(ctx, "greeting", "hello"); err != nil {
	return err
}
// ...
```

The context value is injected based on if a function takes context or
not, and the errors are implicitly handled to interrupt the request if
an error occurs. The error can be handled in PHP or Go.

To use Go types, you can simply bind them to a runner:

```go
rt := runner.New(os.Stdout, runner.Options{RootFS: os.DirFS(".")})
rt.RegisterConstructor("Storage", NewStorage)
```

And this in turn enables `new Storage` to work in PHP. The value behaves
like a PHP `class`, and allows you to use fields or methods from the
struct, as long as they are exported.

This code works with the following implementation:

```go
type Storage interface {
        Set(ctx context.Context, key, value string)
        Get(ctx context.Context, key string) (Record, error)
        All(ctx context.Context) ([]Record, error)
        Len() int64
        Tenant() string
}

func NewStorage(ctx context.Context) (Storage, error) {
	// implement
}
```

While an interface is demonstrated, any type can be used.

The context value is filled from the request, and the errors returned
are promoted to a request error. The error can be handled either by the
VM instance, or by using `try` and `catch` statements in the script.
