# The InternalIterator class

(PHP 8)

## Introduction

Final class that eases implementing [IteratorAggregate](IteratorAggregate.md) for internal classes.

It is not constructible from a script: the constructor is private, and the engine hands instances back from internal classes that expose iteration.

## Class synopsis

```php
final class InternalIterator implements Iterator {

/* Methods */

private function __construct()

public function current(): mixed

public function key(): mixed

public function next(): void

public function rewind(): void

public function valid(): bool

}
```

## Methods

| Method                               | Description                                           |
|--------------------------------------|-------------------------------------------------------|
| `InternalIterator::__construct()`    | Private constructor, disallowing direct instantiation |
| `InternalIterator::current(): mixed` | Return the current element                            |
| `InternalIterator::key(): mixed`     | Return the key of the current element                 |
| `InternalIterator::next(): void`     | Move forward to next element                          |
| `InternalIterator::rewind(): void`   | Rewind the iterator to the first element              |
| `InternalIterator::valid(): bool`    | Checks if current position is valid                   |

## Status

- phpscript does not implement this class. The name is not declared.
- It exists in PHP to serve internal classes, and a script cannot construct one there either, so nothing portable depends on it.
- A host binding that wants to expose a sequence returns a Go slice or an array; the runtime iterates both.
