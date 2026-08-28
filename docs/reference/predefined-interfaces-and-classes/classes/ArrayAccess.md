# The ArrayAccess interface

(PHP 5, PHP 7, PHP 8)

## Introduction

Interface to provide accessing objects as arrays.

A class implementing it can be indexed with `$obj[$key]`, and the four methods it names are what the engine calls for a read, a write, an `isset()` and an `unset()`.

## Interface synopsis

```php
interface ArrayAccess {

/* Methods */

public function offsetExists(mixed $offset): bool

public function offsetGet(mixed $offset): mixed

public function offsetSet(mixed $offset, mixed $value): void

public function offsetUnset(mixed $offset): void

}
```

## Methods

| Method                                                      | Description                            |
|-------------------------------------------------------------|----------------------------------------|
| `ArrayAccess::offsetExists(mixed $offset): bool`            | Whether an offset exists               |
| `ArrayAccess::offsetGet(mixed $offset): mixed`              | Offset to retrieve                     |
| `ArrayAccess::offsetSet(mixed $offset, mixed $value): void` | Assign a value to the specified offset |
| `ArrayAccess::offsetUnset(mixed $offset): void`             | Unset an offset                        |

## Status

- phpscript does not implement this interface. The name is not declared.
- Nothing dispatches through it: there is no overloaded index, so `$obj["k"]` does not call `offsetGet()`. Indexing an object is not how a script reaches its data here.
- Expose the array through a method, or hold it in a property and index the property.
