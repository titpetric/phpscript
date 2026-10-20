# The IteratorAggregate interface

(PHP 5, PHP 7, PHP 8)

## Introduction

Interface to create an external iterator.

A class implementing it returns a separate `Traversable` to iterate, such as an `ArrayIterator` or a generator, and declares none of the five [Iterator](Iterator.md) methods itself.

## Interface synopsis

```php
interface IteratorAggregate extends Traversable {

/* Methods */

public function getIterator(): Traversable

}
```

## Methods

| Method                                          | Description                   |
|-------------------------------------------------|-------------------------------|
| `IteratorAggregate::getIterator(): Traversable` | Retrieve an external iterator |

## Status

- phpscript has no `IteratorAggregate`. The name is not declared.
- The interface uses `extends`. The parser accepts `extends Traversable` and records it for `instanceof`; the clause confers nothing. Traversable declares no method, so flattening it loses nothing.
- There is no interface-based dispatch, so `foreach` never calls `getIterator()`. The usual return values are unavailable too: `ArrayIterator` is not declared and `yield` is a documented won't-build.
- Return an array from a method and loop over that. An array is the sequence type this runtime iterates.
