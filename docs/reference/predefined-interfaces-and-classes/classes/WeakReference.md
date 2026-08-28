# The WeakReference class

(PHP 7 >= 7.4.0, PHP 8)

## Introduction

Reference to an object that does not prevent the object from being destroyed.

Holding one does not raise the object's reference count, so a cache built from weak references does not keep its entries alive. Once the object is gone, `get()` answers null.

## Class synopsis

```php
final class WeakReference {

/* Methods */

public function __construct()

public static function create(object $object): WeakReference

public function get(): ?object

}
```

## Methods

| Method                                                 | Description                              |
|--------------------------------------------------------|------------------------------------------|
| `WeakReference::__construct()`                         | Constructor that disallows instantiation |
| `WeakReference::create(object $object): WeakReference` | Create a new weak reference              |
| `WeakReference::get(): ?object`                        | Get a weakly referenced object           |

## Status

- phpscript does not implement this class. The name is not declared.
- The concept does not carry over. PHP destroys an object when its reference count reaches zero, which is the event a weak reference reports; phpscript objects are Go values collected by the Go garbage collector, and nothing observes the moment one becomes unreachable.
- For a cache whose entries should expire, hold the values in an array and remove them deliberately, or keep the cache on the host side with `SharedMemory`.
