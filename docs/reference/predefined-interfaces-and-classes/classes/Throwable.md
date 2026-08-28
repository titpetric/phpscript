# The Throwable interface

(PHP 7, PHP 8)

## Introduction

Base interface for any object that can be thrown by a `throw` statement, including `Error` and `Exception`.

A PHP class cannot implement it directly: it must extend `Exception` or `Error` instead.

## Interface synopsis

```php
interface Throwable extends Stringable {

/* Methods */

public function getMessage(): string

public function getCode(): int

public function getFile(): string

public function getLine(): int

public function getTrace(): array

public function getTraceAsString(): string

public function getPrevious(): ?Throwable

public function __toString(): string

/* Inherited methods */

public function Stringable::__toString(): string

}
```

## Methods

| Method                                  | Description                                        |
|-----------------------------------------|----------------------------------------------------|
| `Throwable::getMessage(): string`       | Gets the message                                   |
| `Throwable::getCode(): int`             | Gets the exception code                            |
| `Throwable::getFile(): string`          | Gets the file in which the object was created      |
| `Throwable::getLine(): int`             | Gets the line on which the object was instantiated |
| `Throwable::getTrace(): array`          | Gets the stack trace                               |
| `Throwable::getTraceAsString(): string` | Gets the stack trace as a string                   |
| `Throwable::getPrevious(): ?Throwable`  | Returns the previous Throwable                     |
| `Throwable::__toString(): string`       | Gets a string representation of the thrown object  |

## Status

- phpscript supports the name in a catch clause and nowhere else. `catch (Throwable $e)` takes any failure, which is what most code uses the name for.
- The name is not declared as an interface, so `instanceof Throwable` is false, even inside a `catch (Throwable $e)` block that just caught the value.
- The interface uses `extends`. `extends Stringable` is what gives PHP's Throwable its `__toString()`; [Stringable](Stringable.md) is not declared here, and phpscript supports no magic method beyond `__construct` and `__invoke`, so the inherited method has nothing to arrive through.
- There is no exception hierarchy: a catch clause filters on the class name recorded on the value, rather than by walking a parent chain. See [Design decisions](../../../design.md).
