# The WeakMap class

(PHP 8)

## Introduction

Map that accepts objects as keys without contributing to their reference count.

If the map holds the only remaining reference to a key, the object is collected and its entry disappears, which makes it a place to hang data derived from an object without extending the object's lifetime.

## Class synopsis

```php
final class WeakMap implements ArrayAccess, Countable, IteratorAggregate {

/* Methods */

public function count(): int

public function getIterator(): Iterator

public function offsetExists(object $object): bool

public function offsetGet(object $object): mixed

public function offsetSet(object $object, mixed $value): void

public function offsetUnset(object $object): void

}
```

## Methods

| Method                                                   | Description                                      |
|----------------------------------------------------------|--------------------------------------------------|
| `WeakMap::count(): int`                                  | Counts the number of live entries in the map     |
| `WeakMap::getIterator(): Iterator`                       | Retrieve an external iterator                    |
| `WeakMap::offsetExists(object $object): bool`            | Checks whether a certain object is in the map    |
| `WeakMap::offsetGet(object $object): mixed`              | Returns the value pointed to by a certain object |
| `WeakMap::offsetSet(object $object, mixed $value): void` | Updates the map with a new key-value pair        |
| `WeakMap::offsetUnset(object $object): void`             | Removes an entry from the map                    |

## Status

- phpscript does not implement this class. The name is not declared.
- It rests on three things that are unavailable: reference-count semantics, as described for [WeakReference](WeakReference.md), plus [ArrayAccess](ArrayAccess.md) and [IteratorAggregate](IteratorAggregate.md), neither of which dispatches here.
- An object cannot be an array key in either language, so a map keyed by object needs the host. Use an array keyed by an identifier the script already has.
