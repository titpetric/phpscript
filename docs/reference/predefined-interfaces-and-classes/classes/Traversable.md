# The Traversable interface

(PHP 5, PHP 7, PHP 8)

## Introduction

Interface to detect whether a class is traversable using `foreach`.

This is an abstract base interface. No class implements it directly: a class reaches it by implementing [Iterator](Iterator.md) or [IteratorAggregate](IteratorAggregate.md), both of which extend it. Everything PHP considers iterable answers `instanceof Traversable`.

## Interface synopsis

```php
interface Traversable {

}
```

## Status

- phpscript does not declare `Traversable`. The name stays undeclared.
- A class may still write `implements Traversable`, and the parser accepts the name unchecked: a name no `interface` declaration in the same file defines is not a contract.
- The runtime dispatches none of it. `foreach` reads an object's properties, tests no traversability. Implementing this interface does not change a loop's treatment of an object.
- `instanceof Traversable` is false for every value, unless the script declares the interface. A class listing the name then answers true.
