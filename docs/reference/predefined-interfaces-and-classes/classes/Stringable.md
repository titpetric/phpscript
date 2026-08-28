# The Stringable interface

(PHP 8)

## Introduction

Denotes a class as having a `__toString()` method.

PHP adds it implicitly to any class declaring `__toString()`, so the interface exists mainly so a parameter can be typed `string|Stringable` and accept either.

## Interface synopsis

```php
interface Stringable {

/* Methods */

public function __toString(): string

}
```

## Methods

| Method                             | Description                                |
|------------------------------------|--------------------------------------------|
| `Stringable::__toString(): string` | Gets a string representation of the object |

## Status

- phpscript does not implement this interface. The name is not declared.
- The method it names is unavailable: `__toString()` is a magic method, and phpscript supports no magic method beyond `__construct` and `__invoke`. This is a decision rather than a gap; see [Design decisions](../../../design.md).
- An object put where a string is expected renders as an empty string; it does not raise the error PHP raises. Call a method that returns the string.
- [Throwable](Throwable.md) extends this interface in PHP, which is where a thrown object's `__toString()` comes from.
