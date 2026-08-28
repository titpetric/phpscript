# The Traversable interface

(PHP 5, PHP 7, PHP 8)

## Introduction

Interface to detect whether a class is traversable using `foreach`.

This is an abstract base interface. It cannot be implemented on its own: a class reaches it by implementing [Iterator](Iterator.md) or [IteratorAggregate](IteratorAggregate.md), both of which extend it. Every class PHP considers iterable answers `instanceof Traversable`.

## Interface synopsis

```php
interface Traversable {

}
```

## Status

- phpscript does not declare this interface. A class may still write `implements Traversable`, and the name is accepted unchecked, because a name no `interface` declaration in the same file defines is not a contract.
- Nothing dispatches through it. `foreach` reads an object's properties rather than asking whether it is traversable, so implementing this interface changes nothing about how a loop treats an object.
- `instanceof Traversable` is false for every value unless the script declares the interface itself and a class lists it.
