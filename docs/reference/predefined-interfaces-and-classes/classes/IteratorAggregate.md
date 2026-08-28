# The IteratorAggregate interface

(PHP 5, PHP 7, PHP 8)

## Introduction

Interface to create an external iterator.

A class implementing it hands back a separate object to iterate, usually an `ArrayIterator` or a generator, rather than answering the five [Iterator](Iterator.md) methods itself.

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

- phpscript does not implement this interface. The name is not declared.
- The interface uses `extends`. `extends Traversable` parses and is recorded for `instanceof`, and confers nothing; Traversable declares no method, so flattening it loses nothing.
- There is no interface-based dispatch, so `foreach` never calls `getIterator()`. The usual return values are unavailable too: `ArrayIterator` is not declared and `yield` is a documented won't-implement.
- Return an array from a method and loop over that. An array is the sequence type this runtime iterates.
