# Shared memory bindings

`SharedMemory` provides a thread-safe, process-local key/value store and atomic counters. A host can use it for small amounts of state that need to be shared between scripts or HTTP requests handled by the same Go process.

The class is part of the phpscript standard library. No PHP include is needed:

```php
<?php

$shm = new SharedMemory;
```

## Key/value storage

Values are stored as strings:

```php
$shm = new SharedMemory;

$shm->set("status", "ready");

if ($shm->has("status")) {
	echo $shm->get("status"); // ready
}

$shm->delete("status");
echo $shm->get("status"); // an empty string
```

`set($key, $value)` stores a value, `get($key)` retrieves it, and `has($key)` tests whether either a value or counter exists under the key. `delete($key)` removes both the value and counter for that key and returns whether either one existed. `clear()` removes all values and counters.

## Counters

Counters are separate from string values and start at zero. `incr($key)` atomically increments a counter and returns its new integer value. `count($key)` returns the current value as a string:

```php
$shm = new SharedMemory;

echo $shm->incr("requests"); // 1
echo $shm->incr("requests"); // 2
echo $shm->count("requests"); // "2"
```

The available methods are:

| Method            | Returns                                            |
|-------------------|----------------------------------------------------|
| `set($key, $val)` | nothing                                            |
| `get($key)`       | The stored string, or an empty string              |
| `incr($key)`      | The counter's new value, as an int                 |
| `count($key)`     | The counter's current value, as a string           |
| `delete($key)`    | Whether a value or a counter existed under the key |
| `has($key)`       | Whether a value or a counter exists under the key  |
| `clear()`         | nothing                                            |

## Sharing state between runtimes

Without a host-provided instance, each `new SharedMemory` creates an empty store. To retain state between HTTP requests or other PHP runtimes, create one store in Go and add it to every runtime's context:

```go
shm := core.NewSharedMemory()

// Run this while configuring each new runtime.
rt.SetContext(core.SharedMemoryContext(rt.Context(), shm))
stdlib.Register(rt)
```

Constructing `SharedMemory` in any runtime configured this way resolves to the same Go value:

```php
$shm = new SharedMemory;
$shm->incr("requests");
echo $shm->count("requests");
```

The Go store synchronizes concurrent access, so the same instance can safely be used by multiple request runtimes. Despite its name, it is not operating-system shared memory: its contents are not shared between processes and are lost when the host process exits. Use a database or external cache when state must be durable or shared by multiple application processes.

## References

- [Go `SharedMemory` API](https://pkg.go.dev/github.com/titpetric/phpscript@main/stdlib/core#SharedMemory)
- [Shared memory implementation](../../stdlib/core/shared_memory.go)
- [Cross-request test](../../stdlib/core/shared_memory_test.go)
