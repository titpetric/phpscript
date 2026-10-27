# The Traversable interface

(PHP 5, PHP 7, PHP 8)

## Introduction

Interface to detect whether a class is traversable using `foreach`.

This is an abstract base interface. No class implements it on its own: a class reaches it by implementing [Iterator](Iterator.md) or [IteratorAggregate](IteratorAggregate.md), both of which extend it. Every class PHP considers iterable answers `instanceof Traversable`.

## Interface synopsis

```php
interface Traversable {

}
```

## Status

- phpscript has no `Traversable`. The name is not declared.
- A class may still write `implements Traversable`, and the parser accepts the name unchecked, because a name no `interface` declaration in the same file defines is not a contract.
- The runtime dispatches none of it. `foreach` reads an object's properties, tests no traversability, and implementing this interface changes nothing about how a loop treats an object.
- `instanceof Traversable` is false for every value unless the script declares the interface itself and a class lists it.
