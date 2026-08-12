# Shared memory bindings

`PS\SharedMemory` provides a thread-safe, process-local key/value store and
atomic counters. A host can use it for small amounts of state that need to be
shared between scripts or HTTP requests handled by the same Go process.

The class is part of the phpscript standard library. No PHP include is needed:

```php
<?php

$shm = new PS\SharedMemory;
```

## Key/value storage

Values are stored as strings:

```php
$shm = new PS\SharedMemory;

$shm->set("status", "ready");

if ($shm->has("status")) {
	echo $shm->get("status"); // ready
}

$shm->delete("status");
echo $shm->get("status"); // an empty string
```

`set($key, $value)` stores a value, `get($key)` retrieves it, and `has($key)`
tests whether either a value or counter exists under the key. `delete($key)`
removes both the value and counter for that key and returns whether either one
existed. `clear()` removes all values and counters.

## Counters

Counters are separate from string values and start at zero. `incr($key)`
atomically increments a counter and returns its new integer value. `count($key)`
returns the current value as a string:

```php
$shm = new PS\SharedMemory;

echo $shm->incr("requests"); // 1
echo $shm->incr("requests"); // 2
echo $shm->count("requests"); // "2"
```

The available methods are:

```php
namespace PS;

class SharedMemory {
	public function set($key, $value)
	public function get($key)
	public function incr($key)
	public function count($key)
	public function delete($key)
	public function has($key)
	public function clear()
}
```

## Sharing state between runtimes

Without a host-provided instance, each `new PS\SharedMemory` creates an empty
store. To retain state between HTTP requests or other PHP runtimes, create one
store in Go and add it to every runtime's context:

```go
shm := ps.NewSharedMemory()

// Run this while configuring each new runtime.
rt.SetContext(ps.SharedMemoryContext(rt.Context(), shm))
stdlib.Register(rt)
```

Constructing `PS\SharedMemory` in any runtime configured this way resolves to
the same Go value:

```php
$shm = new PS\SharedMemory;
$shm->incr("requests");
echo $shm->count("requests");
```

The Go store synchronizes concurrent access, so the same instance can safely be
used by multiple request runtimes. Despite its name, it is not operating-system
shared memory: its contents are not shared between processes and are lost when
the host process exits. Use a database or external cache when state must be
durable or shared by multiple application processes.

## References

- [Go `SharedMemory` API](https://pkg.go.dev/github.com/titpetric/phpscript@main/stdlib/ps#SharedMemory)
- [Shared memory implementation](../../stdlib/ps/shared_memory.go)
- [Cross-request test](../../stdlib/ps/shared_memory_test.go)
